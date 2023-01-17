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

package manager

import (
	"context"
	"fmt"
	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	networkattachmentv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway loadBalancer according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=k8s.cni.cncf.io,resources=network-attachment-definitions,verbs=get;list;watch;create;update;patch;delete

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
		Owns(&egressgatewayv1alpha1.GatewayVMConfiguration{}).
		Owns(&networkattachmentv1.NetworkAttachmentDefinition{}).
		Complete(r)
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling staticGatewayConfiguration %s/%s", gwConfig.Namespace, gwConfig.Name))

	_, err := controllerutil.CreateOrPatch(ctx, r, gwConfig, func() error {
		// reconcile wireguard keypair
		if err := r.reconcileWireguardKey(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile wireguard key")
			return err
		}

		// reconcile lbconfig
		if err := r.reconcileGatewayLBConfig(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile gateway LB configuration")
			return err
		}

		// reconcile vmconfig
		if err := r.reconcileGatewayVMConfig(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile gateway VM configuration")
			return err
		}
		if err := r.reconcileCNINicConfig(ctx, gwConfig); err != nil {
			log.Error(err, "failed to reconcile nic networkattachment")
			return err
		}
		return nil
	})

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

	log.Info("Deleting VMConfig")
	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if err := r.Delete(ctx, vmConfig); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete existing gateway VM configuration")
			return err
		}
	}

	log.Info("Deleting nic networkattachement")
	networkattachment := &networkattachmentv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getCNINicConfig(gwConfig.Name),
			Namespace: gwConfig.Namespace,
		},
	}
	if err := r.Delete(ctx, networkattachment); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete existing egress cni configuration")
			return err
		}
	}

	log.Info("staticGatewayConfiguration deletion reconciled")
	return nil
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
		if _, ok := secret.Data[consts.WireguardSecretKeyName]; !ok {
			// create new private key
			wgPrivateKey, err := wgtypes.GeneratePrivateKey()
			if err != nil {
				log.Error(err, "failed to generate wireguard private key")
				return err
			}

			secret.Data[consts.WireguardSecretKeyName] = []byte(wgPrivateKey.String())
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
		gwConfig.Status.WireguardPrivateKeySecretRef = &corev1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       secret.Name,
		}

		// Update public key
		gwConfig.Status.WireguardPublicKey = string(secret.Data[consts.WireguardPublicKeyName])
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
		lbConfig.Spec.GatewayVMSSProfile = gwConfig.Spec.GatewayVMSSProfile
		return controllerutil.SetControllerReference(gwConfig, lbConfig, r.Client.Scheme())
	}); err != nil {
		log.Error(err, "failed to reconcile gateway lb configuration")
		return err
	}
	if lbConfig.DeletionTimestamp.IsZero() && lbConfig.Status != nil {
		gwConfig.Status.WireguardServerIP = lbConfig.Status.FrontendIP
		gwConfig.Status.WireguardServerPort = lbConfig.Status.ServerPort
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileGatewayVMConfig(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)

	// check existence of the gatewayVMConfig
	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwConfig.Name,
			Namespace: gwConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, r, vmConfig, func() error {
		vmConfig.Spec.GatewayNodepoolName = gwConfig.Spec.GatewayNodepoolName
		vmConfig.Spec.GatewayVMSSProfile = gwConfig.Spec.GatewayVMSSProfile
		vmConfig.Spec.PublicIpPrefixId = gwConfig.Spec.PublicIpPrefixId
		return controllerutil.SetControllerReference(gwConfig, vmConfig, r.Client.Scheme())
	}); err != nil {
		log.Error(err, "failed to reconcile gateway vm configuration")
		return err
	}

	// Collect status from vmConfig to staticGatewayConfiguration
	if vmConfig.DeletionTimestamp.IsZero() && vmConfig.Status != nil {
		gwConfig.Status.PublicIpPrefix = vmConfig.Status.EgressIpPrefix
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileCNINicConfig(ctx context.Context, gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) error {
	log := log.FromContext(ctx)

	networkattachement := &networkattachmentv1.NetworkAttachmentDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getCNINicConfig(gwConfig.Name),
			Namespace: gwConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r, networkattachement, func() error {
		cniconfig := fmt.Sprintf(
			`{
				"cniVersion": "1.0.0",
				"name": "mynet",
				"plugins": [
					{
						"type": "kube-egress-cni",
						"ipam": {
							"type": "kube-egress-cni-ipam"
						},
						"gatewayName": "%s"
					},
					{
						  "type": "tuning",
						  "sysctl": {
								  "net.ipv4.conf.all.arp_filter": "2"
						  }
					}
				]
			}`, gwConfig.Name)
		networkattachement.Spec.Config = cniconfig
		return controllerutil.SetControllerReference(gwConfig, networkattachement, r.Client.Scheme())
	}); err != nil {
		log.Error(err, "failed to reconcile nic networkattachementconfiguration")
		return err
	}
	return nil
}

func getCNINicConfig(name string) string {
	return name + "-cni"
}
