// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

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

var _ reconcile.Reconciler = &PodEndpointReconciler{}

// PodEndpointReconciler reconciles gateway node network according to a PodEndpoint object
type PodEndpointReconciler struct {
	client.Client
	TickerEvents chan event.GenericEvent
	Netlink      netlinkwrapper.Interface
	NetNS        netnswrapper.Interface
	WgCtrl       wgctrlwrapper.Interface
}

//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=podendpoints,verbs=get;list;watch;
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=podendpoints/status,verbs=get;update;patch
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
func (r *PodEndpointReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Got an event from cleanup ticker
	if req.NamespacedName.Namespace == "" && req.NamespacedName.Name == "" {
		if err := r.cleanUp(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clean up orphaned wireguard peers: %w", err)
		}
	}

	podEndpoint := &egressgatewayv1alpha1.PodEndpoint{}
	if err := r.Get(ctx, req.NamespacedName, podEndpoint); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch PodEndpoint instance")
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
	return r.reconcile(ctx, gwConfig, podEndpoint)
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodEndpointReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Netlink = netlinkwrapper.NewNetLink()
	r.NetNS = netnswrapper.NewNetNS()
	r.WgCtrl = wgctrlwrapper.NewWgCtrl()
	controller, err := ctrl.NewControllerManagedBy(mgr).For(&egressgatewayv1alpha1.PodEndpoint{}).Build(r)
	if err != nil {
		return err
	}
	return controller.Watch(source.Channel(r.TickerEvents, &handler.EnqueueRequestForObject{}))
}

func (r *PodEndpointReconciler) reconcile(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	podEndpoint *egressgatewayv1alpha1.PodEndpoint,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling PodEndpoint")

	nsName := consts.GatewayNetnsName
	gwns, err := r.NetNS.GetNS(nsName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get gateway network namespace %s: %w", nsName, err)
	}
	defer func() {
		if err := gwns.Close(); err != nil {
			log.Error(err, "failed to close gateway namespace")
		}
	}()

	if err := gwns.Do(func(nn ns.NetNS) error {
		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		podPublicKey, err := wgtypes.ParseKey(podEndpoint.Spec.PodPublicKey)
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
					PublicKey:         podPublicKey,
					ReplaceAllowedIPs: true,
					AllowedIPs: []net.IPNet{
						*podIPNet,
					},
				},
			},
		}

		if err := wgClient.ConfigureDevice(getWireguardInterfaceName(gwConfig), wgConfig); err != nil {
			return fmt.Errorf("failed to add peer to wireguard device: %w", err)
		}

		if err := r.addWireguardPeerRoutes(gwConfig, podEndpoint); err != nil {
			return fmt.Errorf("failed to add pod route: %w", err)
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	peerConfigs := []egressgatewayv1alpha1.PeerConfiguration{
		{
			PodEndpoint:   fmt.Sprintf("%s/%s", podEndpoint.Namespace, podEndpoint.Name),
			InterfaceName: getWireguardInterfaceName(gwConfig),
			PublicKey:     podEndpoint.Spec.PodPublicKey,
		},
	}
	if err := r.updateGatewayNodeStatus(ctx, peerConfigs, true /* add */); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Pod wireguard endpoint reconciled")
	return ctrl.Result{}, nil
}

func (r *PodEndpointReconciler) cleanUp(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Cleaning up orphaned wireguard peers")

	podEndpointList := &egressgatewayv1alpha1.PodEndpointList{}
	if err := r.List(ctx, podEndpointList); err != nil {
		return fmt.Errorf("failed to list PodEndpoints: %w", err)
	}
	gwConfigList := &egressgatewayv1alpha1.StaticGatewayConfigurationList{}
	if err := r.List(ctx, gwConfigList); err != nil {
		return fmt.Errorf("failed to list staticGatewayConfigurations: %w", err)
	}
	gwConfigMap := make(map[string]string)
	for _, gwConfig := range gwConfigList.Items {
		gwConfig := gwConfig
		// skip deleting gwConfig, as the wglink will be deleted in staticGatewayConfiguration controller
		if applyToNode(&gwConfig) && gwConfig.ObjectMeta.DeletionTimestamp.IsZero() {
			gwConfigMap[strings.ToLower(fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name))] = getWireguardInterfaceName(&gwConfig)
		}
	}

	// map: gw-namespace-name -> set of peer public keys
	peerMap := make(map[string]map[string]struct{})
	for _, podEndpoint := range podEndpointList.Items {
		if wglinkName, ok := gwConfigMap[strings.ToLower(fmt.Sprintf("%s/%s", podEndpoint.Namespace, podEndpoint.Spec.StaticGatewayConfiguration))]; ok {
			if _, exists := peerMap[wglinkName]; !exists {
				peerMap[wglinkName] = make(map[string]struct{})
			}
			peerMap[wglinkName][podEndpoint.Spec.PodPublicKey] = struct{}{}
		}
	}

	var peersToDelete []egressgatewayv1alpha1.PeerConfiguration
	for _, wglinkName := range gwConfigMap {
		peers, err := r.cleanUpWgLink(ctx, wglinkName, peerMap)
		if err != nil {
			// do not block cleaning up rest namespaces
			log.Error(err, fmt.Sprintf("failed to clean up peers for wgLink %s", wglinkName))
		}
		peersToDelete = append(peersToDelete, peers...)
	}

	if err := r.updateGatewayNodeStatus(ctx, peersToDelete, false /* add */); err != nil {
		return fmt.Errorf("failed to update gateway node status: %w", err)
	}
	log.Info("Wireguard peer cleanup completed")
	return nil
}

func (r *PodEndpointReconciler) cleanUpWgLink(
	ctx context.Context,
	wglinkName string,
	peerMap map[string]map[string]struct{},
) ([]egressgatewayv1alpha1.PeerConfiguration, error) {
	log := log.FromContext(ctx)

	peersToDelete := make([]egressgatewayv1alpha1.PeerConfiguration, 0)

	gwns, err := r.NetNS.GetNS(consts.GatewayNetnsName)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway network namespace %s: %w", consts.GatewayNetnsName, err)
	}
	defer func() { _ = gwns.Close() }()

	if err := gwns.Do(func(nn ns.NetNS) error {
		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		device, err := wgClient.Device(wglinkName)
		if err != nil {
			return fmt.Errorf("failed to get wireguard link configuration: %w", err)
		}

		wgConfig := wgtypes.Config{}
		podIPToDel := make(map[string]bool)
		for i := range device.Peers {
			if _, ok := peerMap[wglinkName][device.Peers[i].PublicKey.String()]; !ok {
				wgConfig.Peers = append(wgConfig.Peers, wgtypes.PeerConfig{
					PublicKey: device.Peers[i].PublicKey,
					Remove:    true,
				})
				for _, ipNet := range device.Peers[i].AllowedIPs {
					podIPToDel[ipNet.IP.String()] = true
				}
				log.Info(fmt.Sprintf("Removing peer %s from wgLink %s", device.Peers[i].PublicKey.String(), wglinkName))
			}
		}
		if len(wgConfig.Peers) > 0 {
			if err := r.deleteWireguardPeerRoutes(wglinkName, podIPToDel); err != nil {
				return fmt.Errorf("failed to delete pod route on wglink %s: %w", wglinkName, err)
			}

			if err := wgClient.ConfigureDevice(wglinkName, wgConfig); err != nil {
				return fmt.Errorf("failed to remove peers from wireguard device %s: %w", wglinkName, err)
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

func (r *PodEndpointReconciler) addWireguardPeerRoutes(
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	podEndpoint *egressgatewayv1alpha1.PodEndpoint,
) error {
	wgLink, err := r.Netlink.LinkByName(getWireguardInterfaceName(gwConfig))
	if err != nil {
		return fmt.Errorf("failed to retrieve wireguard device: %w", err)
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

func (r *PodEndpointReconciler) deleteWireguardPeerRoutes(
	wglinkName string,
	podIPToDel map[string]bool,
) error {
	wgLink, err := r.Netlink.LinkByName(wglinkName)
	if err != nil {
		return fmt.Errorf("failed to get wglink %s: %w", wglinkName, err)
	}

	routes, err := r.Netlink.RouteList(wgLink, netlink.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list routes on wglink %s: %w", wglinkName, err)
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

func (r *PodEndpointReconciler) updateGatewayNodeStatus(
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
