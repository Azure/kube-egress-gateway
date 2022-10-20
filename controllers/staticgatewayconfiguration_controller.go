/*
   MIT License

   Copyright (c) Microsoft Corporation.

   Permission is hereby granted, free of charge, to any person obtaining a copy
   of this software and associated documentation files (the "Software"), to deal
   in the Software without restriction, including without limitation the rights
   to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
   copies of the Software, and to permit persons to whom the Software is
   furnished to do so, subject to the following conditions:

   The above copyright notice and this permission notice shall be included in all
   copies or substantial portions of the Software.

   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
   AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
   OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
   SOFTWARE
*/

package controllers

import (
	"context"
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway loadBalancer according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations/finalizers,verbs=update
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StaticGatewayConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *StaticGatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the StaticGatewayConfiguration instance.
	gwConfig := &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{}
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
		return r.ensureDeleted(ctx, gwConfig)
	}

	return r.reconcile(ctx, gwConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeegressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Owns(&corev1.Secret{}).
		Owns(&kubeegressgatewayv1alpha1.GatewayLBConfiguration{}).
		Complete(r)
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration %s/%s", gwConfig.Namespace, gwConfig.Name))

	if !controllerutil.ContainsFinalizer(gwConfig, SGCFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(gwConfig, SGCFinalizerName)
		err := r.Update(ctx, gwConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
		}
		return ctrl.Result{}, err
	}

	// Make a copy of the original gwConfig to check update
	existing := &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{}
	gwConfig.DeepCopyInto(existing)

	// reconcile wireguard keypair
	if err := r.reconcileWireguardKey(ctx, gwConfig); err != nil {
		log.Error(err, "failed to reconcile wireguard key")
		return ctrl.Result{}, err
	}

	// reconcile lbconfig
	if err := r.reconcileGatewayLBConfig(ctx, gwConfig); err != nil {
		log.Error(err, "failed to reconcile gateway LB configuration")
		return ctrl.Result{}, err
	}

	if !equality.Semantic.DeepEqual(existing, gwConfig) {
		log.Info(fmt.Sprintf("Updating staticGatewayConfiguration %s/%s", gwConfig.Namespace, gwConfig.Name))
		if err := r.Status().Update(ctx, gwConfig); err != nil {
			log.Error(err, "failed to update static gateway configuration")
		}
	}

	log.Info("staticGatewayConfiguration reconciled")
	return ctrl.Result{}, nil
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration deletion %s/%s", gwConfig.Namespace, gwConfig.Name))

	if controllerutil.ContainsFinalizer(gwConfig, SGCFinalizerName) {
		log.Info("Removing finalizer")
		controllerutil.RemoveFinalizer(gwConfig, SGCFinalizerName)
		if err := r.Update(ctx, gwConfig); err != nil {
			log.Error(err, "failed to remove finalizer")
			return ctrl.Result{}, err
		}
	}

	log.Info("staticGatewayConfiguration deletion reconciled")
	return ctrl.Result{}, nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileWireguardKey(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	// check existence of the wireguard secret key
	secretKey := getSubresourceKey(gwConfig)
	secret := &corev1.Secret{}
	if err := r.Get(ctx, *secretKey, secret); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get existing secret %s/%s", secretKey.Namespace, secretKey.Name)
			return err
		} else {
			// secret does not exist, create a new one
			log.Info(fmt.Sprintf("Creating new wireguard key pair for %s/%s", gwConfig.Namespace, gwConfig.Name))

			// create secret
			secret, err = r.createWireguardSecret(ctx, secretKey, gwConfig)
			if err != nil {
				log.Error(err, "failed to create wireguard secret")
				return err
			}
		}
	}

	// Update secret reference
	gwConfig.Status.WireguardPrivateKeySecretRef = &corev1.ObjectReference{
		APIVersion: "v1",
		Kind:       "Secret",
		Name:       secret.Name,
	}

	// Update public key
	wgPrivateKeyByte, ok := secret.Data[WireguardSecretKeyName]
	if !ok {
		return fmt.Errorf("failed to retrieve private key from secret %s/%s", secretKey.Namespace, secretKey.Name)
	}
	wgPrivateKey, err := wgtypes.ParseKey(string(wgPrivateKeyByte))
	if err != nil {
		log.Error(err, "failed to parse private key")
		return err
	}
	wgPublicKey := wgPrivateKey.PublicKey().String()
	gwConfig.Status.WireguardPublicKey = wgPublicKey

	return nil
}

func getSubresourceKey(
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) *types.NamespacedName {
	return &types.NamespacedName{
		Namespace: gwConfig.Namespace,
		Name:      gwConfig.Name,
	}
}

func (r *StaticGatewayConfigurationReconciler) createWireguardSecret(
	ctx context.Context,
	secretKey *types.NamespacedName,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (*corev1.Secret, error) {
	// create new private key
	wgPrivateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	data := map[string][]byte{
		WireguardSecretKeyName: []byte(wgPrivateKey.String()),
	}
	secret := &corev1.Secret{
		Data: data,
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretKey.Name,
			Namespace: secretKey.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(gwConfig, secret, r.Client.Scheme()); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, secret); err != nil {
		return nil, err
	}

	retSecret := &corev1.Secret{}
	if err := r.Get(ctx, *secretKey, retSecret); err != nil {
		return nil, err
	}
	return retSecret, nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileGatewayLBConfig(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	// check existence of the gatewayLBConfig
	lbConfigKey := getSubresourceKey(gwConfig)
	lbConfig := &kubeegressgatewayv1alpha1.GatewayLBConfiguration{}
	if err := r.Get(ctx, *lbConfigKey, lbConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get existing gateway LB configuration %s/%s", lbConfigKey.Namespace, lbConfigKey.Name)
			return err
		} else {
			// lbConfig does not exist, create a new one
			log.Info(fmt.Sprintf("Creating new gateway LB configuration for %s/%s", gwConfig.Namespace, gwConfig.Name))

			// create lbConfig
			lbConfig, err = r.createGatewayLBConfig(ctx, lbConfigKey, gwConfig)
			if err != nil {
				log.Error(err, "failed to create gateway LB configuration")
				return err
			}
		}
	}

	// Collect status from lbConfig to staticGatewayConfiguration
	if lbConfig.Status.FrontendIP != "" {
		gwConfig.Status.WireguardServerIP = lbConfig.Status.FrontendIP
	}
	if lbConfig.Status.ServerPort >= WireguardPortStart && lbConfig.Status.ServerPort < WireguardPortEnd {
		gwConfig.Status.WireguardServerPort = lbConfig.Status.ServerPort
	}

	// Update lbConfig if needed
	if gwConfig.Spec.GatewayNodepoolName != lbConfig.Spec.GatewayNodepoolName ||
		gwConfig.Spec.GatewayVMSSProfile != lbConfig.Spec.GatewayVMSSProfile {
		lbConfig.Spec.GatewayNodepoolName = gwConfig.Spec.GatewayNodepoolName
		lbConfig.Spec.GatewayVMSSProfile = gwConfig.Spec.GatewayVMSSProfile
		if err := r.Update(ctx, lbConfig); err != nil {
			log.Error(err, "failed to update gateway lb configuration")
			return err
		}
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) createGatewayLBConfig(
	ctx context.Context,
	lbConfigKey *types.NamespacedName,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (*kubeegressgatewayv1alpha1.GatewayLBConfiguration, error) {
	lbConfig := &kubeegressgatewayv1alpha1.GatewayLBConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lbConfigKey.Name,
			Namespace: lbConfigKey.Namespace,
		},
		Spec: kubeegressgatewayv1alpha1.GatewayLBConfigurationSpec{
			GatewayNodepoolName: gwConfig.Spec.GatewayNodepoolName,
			GatewayVMSSProfile:  gwConfig.Spec.GatewayVMSSProfile,
		},
	}

	if err := controllerutil.SetControllerReference(gwConfig, lbConfig, r.Client.Scheme()); err != nil {
		return nil, err
	}

	if err := r.Create(ctx, lbConfig); err != nil {
		return nil, err
	}

	retLBConfig := &kubeegressgatewayv1alpha1.GatewayLBConfiguration{}
	if err := r.Get(ctx, *lbConfigKey, retLBConfig); err != nil {
		return nil, err
	}
	return retLBConfig, nil
}
