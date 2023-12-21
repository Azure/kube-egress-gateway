// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway loadBalancer according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
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
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Owns(&corev1.Secret{}).
		Owns(&egressgatewayv1alpha1.GatewayLBConfiguration{}).
		Complete(r)
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration %s/%s", gwConfig.Namespace, gwConfig.Name))

	if err := validate(gwConfig); err != nil {
		return err
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

	prefix := "nil"
	if gwConfig.Status.EgressIpPrefix != "" {
		prefix = gwConfig.Status.EgressIpPrefix
	}
	r.Recorder.Eventf(gwConfig, corev1.EventTypeNormal, "Reconciled", "StaticGatewayConfiguration provisioned with egress prefix %s", prefix)
	log.Info("staticGatewayConfiguration reconciled")
	return err
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration deletion %s/%s", gwConfig.Namespace, gwConfig.Name))

	log.Info("Deleting wireguard key")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if err := r.Delete(ctx, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete existing gateway LB configuration")
			return err
		}
	}

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
		}
	}

	log.Info("staticGatewayConfiguration deletion reconciled")
	return nil
}

func validate(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) error {
	// need to validate either GatewayNodepoolName or GatewayVmssProfile is provided, but not both
	var allErrs field.ErrorList

	if gwConfig.Spec.GatewayNodepoolName == "" && vmssProfileIsEmpty(gwConfig) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewaynodepoolname"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVmssProfile: %#v", gwConfig.Spec.GatewayNodepoolName, gwConfig.Spec.GatewayVmssProfile),
			"Either GatewayNodepoolName or GatewayVmssProfile must be provided"))
	}

	if gwConfig.Spec.GatewayNodepoolName != "" && !vmssProfileIsEmpty(gwConfig) {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewaynodepoolname"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVmssProfile: %#v", gwConfig.Spec.GatewayNodepoolName, gwConfig.Spec.GatewayVmssProfile),
			"Only one of GatewayNodepoolName and GatewayVmssProfile should be provided"))
	}

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

func (r *StaticGatewayConfigurationReconciler) reconcileWireguardKey(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r, secret, func() error {
		// new secret
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

		//update ownerReference
		return controllerutil.SetControllerReference(gwConfig, secret, r.Client.Scheme())
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
