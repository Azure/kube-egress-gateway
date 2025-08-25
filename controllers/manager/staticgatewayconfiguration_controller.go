// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"fmt"
	"os"
	"strings"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/metrics"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway loadBalancer according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	SecretNamespace string
	Recorder        record.EventRecorder
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *StaticGatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the StaticGatewayConfiguration instance.
	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, gwConfig); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch StaticGatewayConfiguration instance")
		return ctrl.Result{}, err
	}

	if !gwConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Clean up staticGatewayConfiguration
		return ctrl.Result{}, r.ensureDeleted(ctx, gwConfig)
	}

	return ctrl.Result{}, r.reconcile(ctx, gwConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	secretPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// no need to trigger reconcile for secrets creation
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return strings.EqualFold(e.ObjectOld.GetNamespace(), r.SecretNamespace)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return strings.EqualFold(e.Object.GetNamespace(), r.SecretNamespace)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Owns(&egressgatewayv1alpha1.GatewayLBConfiguration{}).
		// generated secrets created in the dedicated namespace
		Watches(&corev1.Secret{}, enqueueOwningSGCFromLabels(), builder.WithPredicates(secretPredicate)).
		Complete(r)
}

func enqueueOwningSGCFromLabels() handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
		labels := o.GetLabels()
		if labels == nil {
			return nil
		}

		owningSGCNamespace, foundNamespace := labels[consts.OwningSGCNamespaceLabel]
		owningSGCName, foundName := labels[consts.OwningSGCNameLabel]

		if !foundNamespace || !foundName {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Name:      owningSGCName,
					Namespace: owningSGCNamespace,
				},
			},
		}
	})
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration %s/%s", gwConfig.Namespace, gwConfig.Name))

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"reconcile_static_gateway_configuration",
		"n/a",
		"n/a",
		strings.ToLower(fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name)),
	) // no subscription_id/resource_group for SGC reconciler
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	if err := validate(gwConfig); err != nil {
		r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "InvalidSpec", err.Error())
		return err
	}

	if !controllerutil.ContainsFinalizer(gwConfig, consts.SGCFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(gwConfig, consts.SGCFinalizerName)
		err := r.Update(ctx, gwConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
			return err
		}
	}

	_, err := controllerutil.CreateOrPatch(ctx, r, gwConfig, func() error {
		// reconcile wireguard keypair
		if err := r.reconcileWireguardKey(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile wireguard key")
			r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "ReconcileError", err.Error())
			return err
		}

		// reconcile lbconfig
		if err := r.reconcileGatewayLBConfig(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile gateway LB configuration")
			r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "ReconcileError", err.Error())
			return err
		}

		return nil
	})

	prefix, reconcileStatus := "<pending>", "Reconciling"
	if gwConfig.Status.EgressIpPrefix != "" {
		prefix, reconcileStatus = gwConfig.Status.EgressIpPrefix, "Reconciled"
	}
	r.Recorder.Eventf(gwConfig, corev1.EventTypeNormal, reconcileStatus, "StaticGatewayConfiguration provisioned with egress prefix %s", prefix)
	log.Info("staticGatewayConfiguration reconciled")
	succeeded = true
	return err
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration deletion %s/%s", gwConfig.Namespace, gwConfig.Name))

	if !controllerutil.ContainsFinalizer(gwConfig, consts.SGCFinalizerName) {
		log.Info("gwConfig does not have finalizer, no additional cleanup needed")
		return nil
	}

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"delete_static_gateway_configuration",
		"n/a",
		"n/a",
		strings.ToLower(fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name)),
	) // no subscription_id/resource_group for SGC reconciler
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	secretDeleted := false
	log.Info("Deleting wireguard key")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("sgw-%s", string(gwConfig.UID)),
			Namespace: r.SecretNamespace,
		},
	}
	if err := r.Delete(ctx, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete existing gateway LB configuration")
			return err
		} else {
			secretDeleted = true
		}
	}

	lbConfigDeleted := false
	log.Info("Deleting gateway LB configuration")
	lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if err := r.Delete(ctx, lbConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete existing gateway LB configuration")
			return err
		} else {
			lbConfigDeleted = true
		}
	}

	if secretDeleted && lbConfigDeleted {
		log.Info("Secret and LBConfig are deleted, removing finalizer")
		controllerutil.RemoveFinalizer(gwConfig, consts.SGCFinalizerName)
		if err := r.Update(ctx, gwConfig); err != nil {
			log.Error(err, "failed to remove finalizer")
			return err
		}
	}

	log.Info("staticGatewayConfiguration deletion reconciled")
	succeeded = true
	return nil
}

func validate(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) error {
	// need to validate that one of GatewayNodepoolName, GatewayVmssProfile, or GatewayVmProfile is provided, but not multiple
	var allErrs field.ErrorList
	
	// Check if VM profile is empty
	vmProfileEmpty := vmProfileIsEmpty(gwConfig)
	
	// Check if any of the gateway specifications are provided
	if gwConfig.Spec.GatewayNodepoolName == "" && vmssProfileIsEmpty(gwConfig) && vmProfileEmpty {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVmssProfile: %#v, GatewayVmProfile: %#v", 
			gwConfig.Spec.GatewayNodepoolName, gwConfig.Spec.GatewayVmssProfile, gwConfig.Spec.GatewayVmProfile),
			"One of GatewayNodepoolName, GatewayVmssProfile, or GatewayVmProfile must be provided"))
	}

	// Count how many gateway specifications are provided
	specCount := 0
	if gwConfig.Spec.GatewayNodepoolName != "" {
		specCount++
	}
	if !vmssProfileIsEmpty(gwConfig) {
		specCount++
	}
	if !vmProfileEmpty {
		specCount++
	}
	
	// Only one gateway specification should be provided
	if specCount > 1 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVmssProfile: %#v, GatewayVmProfile: %#v", 
			gwConfig.Spec.GatewayNodepoolName, gwConfig.Spec.GatewayVmssProfile, gwConfig.Spec.GatewayVmProfile),
			"Only one of GatewayNodepoolName, GatewayVmssProfile, or GatewayVmProfile should be provided"))
	}

	// Validate VMSS profile if provided
	if !vmssProfileIsEmpty(gwConfig) {
		if gwConfig.Spec.GatewayVmssProfile.VmssResourceGroup == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("vmssresourcegroup"),
				gwConfig.Spec.GatewayVmssProfile.VmssResourceGroup,
				"Gateway vmss resource group is empty"))
		}
		if gwConfig.Spec.GatewayVmssProfile.VmssName == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("vmssname"),
				gwConfig.Spec.GatewayVmssProfile.VmssName,
				"Gateway vmss name is empty"))
		}
		if gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize < 0 || gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize > 31 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("publicipprefixsize"),
				gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize,
				"Gateway vmss public ip prefix size should be between 0 and 31 inclusively"))
		}
	}
	
	// Validate VM profile if provided
	if !vmProfileIsEmpty(gwConfig) {
		if gwConfig.Spec.GatewayVmProfile.VmResourceGroup == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayVmProfile").Child("vmResourceGroup"),
				gwConfig.Spec.GatewayVmProfile.VmResourceGroup,
				"Gateway vm resource group is empty"))
		}
		if gwConfig.Spec.GatewayVmProfile.VmName == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayVmProfile").Child("vmName"),
				gwConfig.Spec.GatewayVmProfile.VmName,
				"Gateway vm name is empty"))
		}
		if gwConfig.Spec.GatewayVmProfile.PublicIpPrefixSize < 0 || gwConfig.Spec.GatewayVmProfile.PublicIpPrefixSize > 31 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayVmProfile").Child("publicIpPrefixSize"),
				gwConfig.Spec.GatewayVmProfile.PublicIpPrefixSize,
				"Gateway vm public ip prefix size should be between 0 and 31 inclusively"))
		}
	}

	if !gwConfig.Spec.ProvisionPublicIps && gwConfig.Spec.PublicIpPrefixId != "" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("publicipprefixid"),
			gwConfig.Spec.PublicIpPrefixId,
			"PublicIpPrefixId should be empty when ProvisionPublicIps is false"))
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "egressgateway.kubernetes.azure.com", Kind: "StaticGatewayConfiguration"},
		gwConfig.Name, allErrs)
}

func vmssProfileIsEmpty(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	return gwConfig.Spec.GatewayVmssProfile.VmssResourceGroup == "" &&
		gwConfig.Spec.GatewayVmssProfile.VmssName == "" &&
		gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize == 0
}

func vmProfileIsEmpty(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	return gwConfig.Spec.GatewayVmProfile.VmResourceGroup == "" &&
		gwConfig.Spec.GatewayVmProfile.VmName == "" &&
		gwConfig.Spec.GatewayVmProfile.PublicIpPrefixSize == 0
}


func gatewayPoolProfileIsEmpty(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	return gwConfig.Spec.GatewayPoolProfile.Type == "" &&
		gwConfig.Spec.GatewayPoolProfile.Name == "" &&
		gwConfig.Spec.GatewayPoolProfile.ResourceGroup == "" &&
		gwConfig.Spec.GatewayPoolProfile.PublicIpPrefixSize == 0
}

func (r *StaticGatewayConfigurationReconciler) reconcileWireguardKey(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("sgw-%s", string(gwConfig.UID)),
			Namespace: r.SecretNamespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r, secret, func() error {
		if secret.Labels == nil {
			secret.Labels = make(map[string]string)
		}
		if sgcNS, ok := secret.Labels[consts.OwningSGCNamespaceLabel]; !ok || sgcNS != gwConfig.Namespace {
			secret.Labels[consts.OwningSGCNamespaceLabel] = gwConfig.Namespace
		}
		if sgcName, ok := secret.Labels[consts.OwningSGCNameLabel]; !ok || sgcName != gwConfig.Name {
			secret.Labels[consts.OwningSGCNameLabel] = gwConfig.Name
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		if _, ok := secret.Data[consts.WireguardPrivateKeyName]; !ok {
			// create new private key
			wgPrivateKey, err := wgtypes.GeneratePrivateKey()
			if err != nil {
				log.Error(err, "failed to generate wireguard private key")
				return err
			}

			secret.Data[consts.WireguardPrivateKeyName] = []byte(wgPrivateKey.String())
			secret.Data[consts.WireguardPublicKeyName] = []byte(wgPrivateKey.PublicKey().String())
		}

		return nil
	}); err != nil {
		log.Error(err, "failed to reconcile wireguard keypair secret")
		return err
	}
	if secret.DeletionTimestamp.IsZero() {
		// Update secret reference
		gwConfig.Status.PrivateKeySecretRef = &corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       secret.Name,
			Namespace:  secret.Namespace,
		}

		// Update public key
		gwConfig.Status.PublicKey = string(secret.Data[consts.WireguardPublicKeyName])
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileGatewayLBConfig(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	// check existence of the gatewayLBConfig
	lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, r, lbConfig, func() error {
		lbConfig.Spec.GatewayNodepoolName = gwConfig.Spec.GatewayNodepoolName
		lbConfig.Spec.GatewayVmssProfile = gwConfig.Spec.GatewayVmssProfile
		lbConfig.Spec.ProvisionPublicIps = gwConfig.Spec.ProvisionPublicIps
		lbConfig.Spec.PublicIpPrefixId = gwConfig.Spec.PublicIpPrefixId
		return controllerutil.SetControllerReference(gwConfig, lbConfig, r.Client.Scheme())
	}); err != nil {
		log.Error(err, "failed to reconcile gateway lb configuration")
		return err
	}
	if lbConfig.DeletionTimestamp.IsZero() && lbConfig.Status != nil {
		gwConfig.Status.Ip = lbConfig.Status.FrontendIp
		gwConfig.Status.Port = lbConfig.Status.ServerPort
		gwConfig.Status.EgressIpPrefix = lbConfig.Status.EgressIpPrefix
	}

	return nil
}
