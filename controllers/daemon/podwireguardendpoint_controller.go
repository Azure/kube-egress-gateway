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

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
)

var _ reconcile.Reconciler = &PodWireguardEndpointReconciler{}

// PodWireguardEndpointReconciler reconciles gateway node network according to a PodWireguardEndpoint object
type PodWireguardEndpointReconciler struct {
	client.Client
	TickerEvents chan event.GenericEvent
	Netlink      netlinkwrapper.Interface
	NetNS        netnswrapper.Interface
	WgCtrl       wgctrlwrapper.Interface
}

//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=podwireguardendpoints,verbs=get;list;watch;
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=podwireguardendpoints/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaystatuses,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StaticGatewayConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *PodWireguardEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Got an event from cleanup ticker
	if req.NamespacedName.Namespace == "" && req.NamespacedName.Name == "" {
		if err := r.cleanUp(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clean up orphaned wireguard peers: %w", err)
		}
	}

	podEndpoint := &egressgatewayv1alpha1.PodWireguardEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, podEndpoint); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch PodWireguardEndpoint instance")
		return ctrl.Result{}, err
	}

	gwConfigKey := types.NamespacedName{
		Namespace: podEndpoint.Namespace,
		Name:      podEndpoint.Spec.StaticGatewayConfiguration,
	}
	// Fetch the StaticGatewayConfiguration instance.
	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.Get(ctx, gwConfigKey, gwConfig); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch StaticGatewayConfiguration(%s/%s): %w", gwConfigKey.Namespace, gwConfigKey.Name, err)
	}

	if !applyToNode(gwConfig) {
		// gwConfig does not apply to this node
		return ctrl.Result{}, nil
	}

	// Reconcile wireguard peer
	return r.reconcile(ctx, getGatewayNamespaceName(gwConfig), podEndpoint)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodWireguardEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Netlink = netlinkwrapper.NewNetLink()
	r.NetNS = netnswrapper.NewNetNS()
	r.WgCtrl = wgctrlwrapper.NewWgCtrl()
	controller, err := ctrl.NewControllerManagedBy(mgr).For(&egressgatewayv1alpha1.PodWireguardEndpoint{}).Build(r)
	if err != nil {
		return err
	}
	return controller.Watch(&source.Channel{Source: r.TickerEvents}, &handler.EnqueueRequestForObject{})
}

func (r *PodWireguardEndpointReconciler) reconcile(
	ctx context.Context,
	gwNamespaceName string,
	podEndpoint *egressgatewayv1alpha1.PodWireguardEndpoint,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling PodWireguardEndpoint")

	gwns, err := r.NetNS.GetNS(gwNamespaceName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get gateway network namespace %s: %w", gwNamespaceName, err)
	}
	defer gwns.Close()

	if err := gwns.Do(func(nn ns.NetNS) error {
		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		podWireguardPublicKey, err := wgtypes.ParseKey(podEndpoint.Spec.PodWireguardPublicKey)
		if err != nil {
			return fmt.Errorf("failed to parse pod wireguard public key: %w", err)
		}

		_, podIPNet, err := net.ParseCIDR(podEndpoint.Spec.PodIpAddress)
		if err != nil {
			return fmt.Errorf("failed to parse pod IPv4 address: %w", err)
		}

		wgConfig := wgtypes.Config{
			Peers: []wgtypes.PeerConfig{
				{
					PublicKey:         podWireguardPublicKey,
					ReplaceAllowedIPs: true,
					AllowedIPs: []net.IPNet{
						*podIPNet,
					},
				},
			},
		}

		if err := wgClient.ConfigureDevice(consts.WireguardLinkName, wgConfig); err != nil {
			return fmt.Errorf("failed to add peer to wireguard device: %w", err)
		}

		if err := r.addWireguardPeerRoutes(podEndpoint); err != nil {
			return fmt.Errorf("failed to add pod route: %w", err)
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	peerConfigs := []egressgatewayv1alpha1.PeerConfiguration{
		{
			PodWireguardEndpoint: fmt.Sprintf("%s/%s", podEndpoint.Namespace, podEndpoint.Name),
			NetnsName:            gwNamespaceName,
			PublicKey:            podEndpoint.Spec.PodWireguardPublicKey,
		},
	}
	if err := r.updateGatewayNodeStatus(ctx, peerConfigs, true /* add */); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Pod wireguard endpoint reconciled")
	return ctrl.Result{}, nil
}

func (r *PodWireguardEndpointReconciler) cleanUp(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Cleaning up orphaned wireguard peers")

	podEndpointList := &egressgatewayv1alpha1.PodWireguardEndpointList{}
	if err := r.List(ctx, podEndpointList); err != nil {
		return fmt.Errorf("failed to list podWireguardEndpoints: %w", err)
	}
	gwConfigList := &egressgatewayv1alpha1.StaticGatewayConfigurationList{}
	if err := r.List(ctx, gwConfigList); err != nil {
		return fmt.Errorf("failed to list staticGatewayConfigurations: %w", err)
	}
	gwConfigMap := make(map[string]string)
	for _, gwConfig := range gwConfigList.Items {
		gwConfig := gwConfig
		if applyToNode(&gwConfig) {
			gwConfigMap[fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name)] = getGatewayNamespaceName(&gwConfig)
		}
	}

	// map: gw-namespace-name -> set of peer public keys
	peerMap := make(map[string]map[string]bool)
	for _, podEndpoint := range podEndpointList.Items {
		if nsName, ok := gwConfigMap[fmt.Sprintf("%s/%s", podEndpoint.Namespace, podEndpoint.Spec.StaticGatewayConfiguration)]; ok {
			if _, exists := peerMap[nsName]; !exists {
				peerMap[nsName] = make(map[string]bool)
			}
			peerMap[nsName][podEndpoint.Spec.PodWireguardPublicKey] = true
		}
	}

	var peersToDelete []egressgatewayv1alpha1.PeerConfiguration
	for _, ns := range gwConfigMap {
		peers, err := r.cleanUpNetNS(ctx, ns, peerMap)
		if err != nil {
			// do not block cleaning up rest namespaces
			log.Error(err, fmt.Sprintf("failed to clean up peers in namespace %s", ns))
		}
		peersToDelete = append(peersToDelete, peers...)
	}

	if err := r.updateGatewayNodeStatus(ctx, peersToDelete, false /* add */); err != nil {
		return fmt.Errorf("failed to update gateway node status: %w", err)
	}
	log.Info("Wireguard peer cleanup completed")
	return nil
}

func (r *PodWireguardEndpointReconciler) cleanUpNetNS(
	ctx context.Context,
	nsName string,
	peerMap map[string]map[string]bool,
) ([]egressgatewayv1alpha1.PeerConfiguration, error) {
	log := log.FromContext(ctx)

	peersToDelete := make([]egressgatewayv1alpha1.PeerConfiguration, 0)

	gwns, err := r.NetNS.GetNS(nsName)
	if err != nil {
		// do not return error to continue cleanup
		return nil, fmt.Errorf("failed to get gateway network namespace %s", nsName)
	}
	defer gwns.Close()

	if err := gwns.Do(func(nn ns.NetNS) error {
		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		device, err := wgClient.Device(consts.WireguardLinkName)
		if err != nil {
			return fmt.Errorf("failed to get wireguard link configuration: %w", err)
		}

		wgConfig := wgtypes.Config{}
		podIPToDel := make(map[string]bool)
		for i := range device.Peers {
			if _, ok := peerMap[nsName][device.Peers[i].PublicKey.String()]; !ok {
				wgConfig.Peers = append(wgConfig.Peers, wgtypes.PeerConfig{
					PublicKey: device.Peers[i].PublicKey,
					Remove:    true,
				})
				for _, ipNet := range device.Peers[i].AllowedIPs {
					podIPToDel[ipNet.IP.String()] = true
				}
				log.Info(fmt.Sprintf("Removing peer %s from gw namespace %s", device.Peers[i].PublicKey.String(), nsName))
			}
		}
		if len(wgConfig.Peers) > 0 {
			if err := r.deleteWireguardPeerRoutes(podIPToDel); err != nil {
				return fmt.Errorf("failed to delete pod route: %w", err)
			}

			if err := wgClient.ConfigureDevice(consts.WireguardLinkName, wgConfig); err != nil {
				return fmt.Errorf("failed to remove peers from wireguard device: %w", err)
			}

			for _, peer := range wgConfig.Peers {
				peersToDelete = append(peersToDelete, egressgatewayv1alpha1.PeerConfiguration{PublicKey: peer.PublicKey.String()})
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return peersToDelete, nil
}

func (r *PodWireguardEndpointReconciler) addWireguardPeerRoutes(
	podEndpoint *egressgatewayv1alpha1.PodWireguardEndpoint,
) error {
	wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
	if err != nil {
		return fmt.Errorf("failed to retrive wireguard device: %w", err)
	}

	_, dst, err := net.ParseCIDR(podEndpoint.Spec.PodIpAddress)
	if err != nil {
		return fmt.Errorf("failed to parse pod ip net %s: %w", podEndpoint.Spec.PodIpAddress, err)
	}

	route := &netlink.Route{
		LinkIndex: wgLink.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       dst,
	}
	if err := r.Netlink.RouteReplace(route); err != nil {
		return fmt.Errorf("failed to add route %s: %w", route, err)
	}

	return nil
}

func (r *PodWireguardEndpointReconciler) deleteWireguardPeerRoutes(
	podIPToDel map[string]bool,
) error {
	wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
	if err != nil {
		return fmt.Errorf("failed to retrive wireguard device wg0: %w", err)
	}

	routes, err := r.Netlink.RouteList(wgLink, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	for _, route := range routes {
		route := route
		if _, ok := podIPToDel[route.Dst.IP.String()]; ok {
			if err := r.Netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("failed to delete route %s: %w", route, err)
			}
		}
	}

	return nil
}

func (r *PodWireguardEndpointReconciler) updateGatewayNodeStatus(
	ctx context.Context,
	peerConfigs []egressgatewayv1alpha1.PeerConfiguration,
	add bool,
) error {
	log := log.FromContext(ctx)
	gwStatusKey := types.NamespacedName{
		Namespace: os.Getenv(consts.PodNamespaceEnvKey),
		Name:      os.Getenv(consts.NodeNameEnvKey),
	}

	gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
	if err := r.Get(ctx, gwStatusKey, gwStatus); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Error(err, "failed to get existing gateway status object %s/%s", gwStatusKey.Namespace, gwStatusKey.Name)
			return err
		} else {
			if !add {
				// ignore creating object during cleanup
				return nil
			}

			// gwStatus does not exist, create a new one
			log.Info(fmt.Sprintf("Creating new gateway status(%s/%s)", gwStatusKey.Namespace, gwStatusKey.Name))

			node := &corev1.Node{}
			if err := r.Get(ctx, types.NamespacedName{Name: os.Getenv(consts.NodeNameEnvKey)}, node); err != nil {
				return fmt.Errorf("failed to get current node: %w", err)
			}

			gwStatus := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwStatusKey.Name,
					Namespace: gwStatusKey.Namespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyPeerConfigurations: peerConfigs,
				},
			}
			if err := controllerutil.SetOwnerReference(node, gwStatus, r.Client.Scheme()); err != nil {
				return fmt.Errorf("failed to set gwStatus owner reference to node: %w", err)
			}
			log.Info("Creating new gateway status object")
			if err := r.Create(ctx, gwStatus); err != nil {
				return fmt.Errorf("failed to create gwStatus object: %w", err)
			}
		}
	} else {
		changed := false
		peerMap := make(map[string]*egressgatewayv1alpha1.PeerConfiguration)
		for _, peerConfig := range gwStatus.Spec.ReadyPeerConfigurations {
			peerConfig := peerConfig
			peerMap[peerConfig.PublicKey] = &peerConfig
		}
		for i, peerConfig := range peerConfigs {
			if _, ok := peerMap[peerConfig.PublicKey]; ok {
				if !add {
					delete(peerMap, peerConfig.PublicKey)
					changed = true
				}
			} else {
				if add {
					peerMap[peerConfig.PublicKey] = &peerConfigs[i]
					changed = true
				}
			}
		}
		if changed {
			var peers []egressgatewayv1alpha1.PeerConfiguration
			for _, peerConfig := range peerMap {
				peers = append(peers, *peerConfig)
			}
			gwStatus.Spec.ReadyPeerConfigurations = peers
			log.Info("Updating gateway status object")
			if err := r.Update(ctx, gwStatus); err != nil {
				return fmt.Errorf("failed to update gwStatus object: %w", err)
			}
		}
	}
	return nil
}
