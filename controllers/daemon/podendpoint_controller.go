// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
)

var _ reconcile.Reconciler = &PodEndpointReconciler{}

var errInvalidPodEndpoint = errors.New("invalid PodEndpoint")

// PodEndpointReconciler reconciles gateway node network according to a PodEndpoint object
type PodEndpointReconciler struct {
	client.Client
	TickerEvents chan event.GenericEvent
	Netlink      netlinkwrapper.Interface
	NetNS        netnswrapper.Interface
	WgCtrl       wgctrlwrapper.Interface
}

// PeerUpdateOperation defines the type of operation to perform on peer configurations
type PeerUpdateOperation string

const (
	// PeerUpdateOpAdd adds the provided peer configurations to the existing list
	PeerUpdateOpAdd PeerUpdateOperation = "ADD"
	// PeerUpdateOpDelete removes all peers except those in the provided list
	PeerUpdateOpDelete PeerUpdateOperation = "DELETE"
)

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

	if err := r.validatePodEndpoint(ctx, podEndpoint); err != nil {
		if !errors.Is(err, errInvalidPodEndpoint) {
			return ctrl.Result{}, err
		}
		log.Error(err, "rejecting invalid PodEndpoint")
		if cleanupErr := r.cleanUp(ctx); cleanupErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to revoke invalid PodEndpoint state: %w", cleanupErr)
		}
		return ctrl.Result{}, nil
	}
	if err := r.validateUniquePodIPClaim(ctx, podEndpoint); err != nil {
		if !errors.Is(err, errInvalidPodEndpoint) {
			return ctrl.Result{}, err
		}
		log.Error(err, "rejecting conflicting PodEndpoint IP claim")
		if cleanupErr := r.cleanUp(ctx); cleanupErr != nil {
			return ctrl.Result{}, fmt.Errorf("failed to revoke conflicting PodEndpoint state: %w", cleanupErr)
		}
		return ctrl.Result{}, nil
	}

	gwConfigKey := types.NamespacedName{
		Namespace: podEndpoint.Namespace,
		Name:      podEndpoint.Spec.StaticGatewayConfiguration,
	}
	// Fetch the StaticGatewayConfiguration instance.
	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.Get(ctx, gwConfigKey, gwConfig); err != nil {
		if apierrors.IsNotFound(err) {
			if cleanupErr := r.cleanUp(ctx); cleanupErr != nil {
				return ctrl.Result{}, fmt.Errorf("failed to revoke PodEndpoint state for missing StaticGatewayConfiguration: %w", cleanupErr)
			}
		}
		return ctrl.Result{}, fmt.Errorf("failed to fetch StaticGatewayConfiguration(%s/%s): %w", gwConfigKey.Namespace, gwConfigKey.Name, err)
	}

	if !applyToNode(gwConfig) {
		if err := r.cleanUp(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to revoke PodEndpoint state from this node: %w", err)
		}
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
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.PodEndpoint{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(func(_ context.Context, pod client.Object) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: client.ObjectKeyFromObject(pod)}}
		}), builder.WithPredicates(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return usesStaticEgressGateway(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return usesStaticEgressGateway(e.ObjectOld) || usesStaticEgressGateway(e.ObjectNew)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return usesStaticEgressGateway(e.Object)
			},
			GenericFunc: func(event.GenericEvent) bool {
				return false
			},
		})).
		Build(r)
	if err != nil {
		return err
	}
	return controller.Watch(source.Channel(r.TickerEvents, &handler.EnqueueRequestForObject{}))
}

func usesStaticEgressGateway(object client.Object) bool {
	_, found := object.GetAnnotations()[consts.CNIGatewayAnnotationKey]
	return found
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

		route, routeCreated, err := r.ensureWireguardPeerRoute(gwConfig, podIPNet)
		if err != nil {
			return fmt.Errorf("failed to ensure pod route: %w", err)
		}
		if err := wgClient.ConfigureDevice(getWireguardInterfaceName(gwConfig), wgConfig); err != nil {
			if routeCreated {
				if routeErr := r.Netlink.RouteDel(route); routeErr != nil {
					return fmt.Errorf(
						"failed to add peer to wireguard device: %w; also failed to roll back route %s: %v",
						err,
						route,
						routeErr,
					)
				}
			}
			return fmt.Errorf("failed to add peer to wireguard device: %w", err)
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
	if err := r.updateGatewayNodeStatus(ctx, peerConfigs, PeerUpdateOpAdd); err != nil {
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

	validPodEndpoints := make([]*egressgatewayv1alpha1.PodEndpoint, 0, len(podEndpointList.Items))
	ipClaims := make(map[string][]*egressgatewayv1alpha1.PodEndpoint)
	for _, podEndpoint := range podEndpointList.Items {
		podEndpoint := podEndpoint
		if err := r.validatePodEndpoint(ctx, &podEndpoint); err != nil {
			if !errors.Is(err, errInvalidPodEndpoint) {
				return fmt.Errorf("failed to validate PodEndpoint %s/%s: %w", podEndpoint.Namespace, podEndpoint.Name, err)
			}
			log.Error(err, "excluding invalid PodEndpoint from expected WireGuard peers",
				"namespace", podEndpoint.Namespace, "name", podEndpoint.Name)
			continue
		}
		podIP, err := parseCanonicalPodIPv4CIDR(podEndpoint.Spec.PodIpAddress)
		if err != nil {
			return fmt.Errorf("failed to parse validated PodEndpoint IP %s/%s: %w", podEndpoint.Namespace, podEndpoint.Name, err)
		}
		validPodEndpoints = append(validPodEndpoints, &podEndpoint)
		ipClaims[podIP.String()] = append(ipClaims[podIP.String()], &podEndpoint)
	}

	// map: wg-link-name -> set of peer public keys
	peerMap := make(map[string]map[string]struct{})
	for _, podEndpoint := range validPodEndpoints {
		podIP, err := parseCanonicalPodIPv4CIDR(podEndpoint.Spec.PodIpAddress)
		if err != nil {
			return fmt.Errorf("failed to parse validated PodEndpoint IP %s/%s: %w", podEndpoint.Namespace, podEndpoint.Name, err)
		}
		if claimants := ipClaims[podIP.String()]; len(claimants) > 1 {
			log.Error(
				fmt.Errorf("%w: IP %s is claimed by %d PodEndpoints", errInvalidPodEndpoint, podIP, len(claimants)),
				"excluding conflicting PodEndpoint from expected WireGuard peers",
				"namespace", podEndpoint.Namespace,
				"name", podEndpoint.Name,
			)
			continue
		}
		if wglinkName, ok := gwConfigMap[strings.ToLower(fmt.Sprintf("%s/%s", podEndpoint.Namespace, podEndpoint.Spec.StaticGatewayConfiguration))]; ok {
			if _, exists := peerMap[wglinkName]; !exists {
				peerMap[wglinkName] = make(map[string]struct{})
			}
			peerMap[wglinkName][podEndpoint.Spec.PodPublicKey] = struct{}{}
		}
	}

	var keep []egressgatewayv1alpha1.PeerConfiguration
	for _, wglinkName := range gwConfigMap {
		peers, err := r.cleanUpWgLink(ctx, wglinkName, peerMap)
		if err != nil {
			// do not block cleaning up rest namespaces
			log.Error(err, fmt.Sprintf("failed to clean up peers for wgLink %s", wglinkName))
		}
		keep = append(keep, peers...)
	}

	if err := r.updateGatewayNodeStatus(ctx, keep, PeerUpdateOpDelete); err != nil {
		return fmt.Errorf("failed to update gateway node status: %w", err)
	}
	log.Info("Wireguard peer cleanup completed")
	return nil
}

// cleanUpWgLink removes orphaned wireguard peers from the specified interface.
// It returns a list of PeerConfigurations to keep based on the input peerMap.
func (r *PodEndpointReconciler) cleanUpWgLink(
	ctx context.Context,
	wglinkName string,
	peerMap map[string]map[string]struct{},
) ([]egressgatewayv1alpha1.PeerConfiguration, error) {
	log := log.FromContext(ctx)

	peersToKeep := make([]egressgatewayv1alpha1.PeerConfiguration, 0)

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
		podIPToKeep := make(map[string]bool)
		for i := range device.Peers {
			if _, ok := peerMap[wglinkName][device.Peers[i].PublicKey.String()]; !ok {
				wgConfig.Peers = append(wgConfig.Peers, wgtypes.PeerConfig{
					PublicKey: device.Peers[i].PublicKey,
					Remove:    true,
				})
				for _, ipNet := range device.Peers[i].AllowedIPs {
					podIPToDel[ipNet.String()] = true
				}
				log.Info(fmt.Sprintf("Removing peer %s from wgLink %s", device.Peers[i].PublicKey.String(), wglinkName))
			} else {
				peersToKeep = append(peersToKeep, egressgatewayv1alpha1.PeerConfiguration{PublicKey: device.Peers[i].PublicKey.String()})
				for _, ipNet := range device.Peers[i].AllowedIPs {
					podIPToKeep[ipNet.String()] = true
				}
			}
		}
		if len(wgConfig.Peers) > 0 {
			for podIP := range podIPToKeep {
				delete(podIPToDel, podIP)
			}
			if len(podIPToDel) > 0 {
				if err := r.deleteWireguardPeerRoutes(wglinkName, podIPToDel); err != nil {
					return fmt.Errorf("failed to delete pod route on wglink %s: %w", wglinkName, err)
				}
			}

			if err := wgClient.ConfigureDevice(wglinkName, wgConfig); err != nil {
				return fmt.Errorf("failed to remove peers from wireguard device %s: %w", wglinkName, err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return peersToKeep, nil
}

func (r *PodEndpointReconciler) ensureWireguardPeerRoute(
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	dst *net.IPNet,
) (*netlink.Route, bool, error) {
	wgLink, err := r.Netlink.LinkByName(getWireguardInterfaceName(gwConfig))
	if err != nil {
		return nil, false, fmt.Errorf("failed to retrieve wireguard device: %w", err)
	}

	owned, err := r.inspectWireguardRouteOwnership(wgLink, dst)
	if err != nil {
		return nil, false, err
	}
	if owned {
		return &netlink.Route{
			LinkIndex: wgLink.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Dst:       dst,
		}, false, nil
	}

	route := &netlink.Route{
		LinkIndex: wgLink.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
		Dst:       dst,
	}
	if err := r.Netlink.RouteAdd(route); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			owned, inspectErr := r.inspectWireguardRouteOwnership(wgLink, dst)
			if inspectErr != nil {
				return nil, false, inspectErr
			}
			if owned {
				return route, false, nil
			}
			return nil, false, fmt.Errorf("route %s already exists without matching this WireGuard interface", dst)
		}
		return nil, false, fmt.Errorf("failed to add route %s: %w", route, err)
	}

	return route, true, nil
}

// inspectWireguardRouteOwnership checks every Static Egress Gateway WireGuard interface
// for a route that overlaps the requested destination. It reports the route as owned only
// when an identical destination is already installed on the intended interface; any route
// on another interface or with a broader overlapping prefix is treated as a conflict.
func (r *PodEndpointReconciler) inspectWireguardRouteOwnership(
	targetLink netlink.Link,
	target *net.IPNet,
) (bool, error) {
	links, err := r.Netlink.LinkList()
	if err != nil {
		return false, fmt.Errorf("failed to list links while checking route ownership: %w", err)
	}
	for _, link := range links {
		if !strings.HasPrefix(link.Attrs().Name, consts.WiregaurdLinkNamePrefix) {
			continue
		}
		routes, err := r.Netlink.RouteList(link, netlink.FAMILY_V4)
		if err != nil {
			return false, fmt.Errorf("failed to list routes on WireGuard link %s: %w", link.Attrs().Name, err)
		}
		for _, route := range routes {
			if route.Dst == nil || !cidrsOverlap(route.Dst, target) {
				continue
			}
			if link.Attrs().Index == targetLink.Attrs().Index && route.Dst.String() == target.String() {
				return true, nil
			}
			return false, fmt.Errorf(
				"route %s on WireGuard link %s overlaps claimed route %s on %s",
				route.Dst,
				link.Attrs().Name,
				target,
				targetLink.Attrs().Name,
			)
		}
	}
	return false, nil
}

func cidrsOverlap(left, right *net.IPNet) bool {
	return left.Contains(right.IP) || right.Contains(left.IP)
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
		if route.Dst != nil && podIPToDel[route.Dst.String()] {
			if err := r.Netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("failed to delete route %s: %w", route, err)
			}
		}
	}

	return nil
}

// validatePodEndpoint prevents namespace-scoped PodEndpoint writers from injecting arbitrary
// WireGuard peers and routes. Before the daemon applies the endpoint, it verifies that:
//   - the CNI manager attested the current spec generation and Pod UID;
//   - the controller owner reference identifies that same Pod;
//   - the Pod still exists with the attested UID and is not terminating;
//   - the claimed address is a canonical IPv4 /32; and
//   - the claimed address matches the Pod IP reported by Kubernetes when available.
func (r *PodEndpointReconciler) validatePodEndpoint(
	ctx context.Context,
	podEndpoint *egressgatewayv1alpha1.PodEndpoint,
) error {
	if podEndpoint.Status.PodUID == "" {
		return fmt.Errorf("%w: status.podUID is empty", errInvalidPodEndpoint)
	}
	if podEndpoint.Generation <= 0 || podEndpoint.Status.ObservedGeneration != podEndpoint.Generation {
		return fmt.Errorf(
			"%w: generation %d has not been attested, observed generation is %d",
			errInvalidPodEndpoint,
			podEndpoint.Generation,
			podEndpoint.Status.ObservedGeneration,
		)
	}

	controllerRef := metav1.GetControllerOf(podEndpoint)
	if controllerRef == nil ||
		controllerRef.APIVersion != corev1.SchemeGroupVersion.String() ||
		controllerRef.Kind != "Pod" ||
		controllerRef.Name != podEndpoint.Name ||
		controllerRef.UID != podEndpoint.Status.PodUID {
		return fmt.Errorf("%w: controller owner reference does not match the attested Pod", errInvalidPodEndpoint)
	}

	pod := &corev1.Pod{}
	podKey := types.NamespacedName{Namespace: podEndpoint.Namespace, Name: podEndpoint.Name}
	if err := r.Get(ctx, podKey, pod); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("%w: Pod %s/%s does not exist", errInvalidPodEndpoint, podKey.Namespace, podKey.Name)
		}
		return fmt.Errorf("failed to fetch Pod %s/%s: %w", podKey.Namespace, podKey.Name, err)
	}
	if !pod.DeletionTimestamp.IsZero() {
		return fmt.Errorf("%w: Pod %s/%s is terminating", errInvalidPodEndpoint, pod.Namespace, pod.Name)
	}
	if pod.UID != podEndpoint.Status.PodUID {
		return fmt.Errorf(
			"%w: live Pod UID %s does not match attested UID %s",
			errInvalidPodEndpoint,
			pod.UID,
			podEndpoint.Status.PodUID,
		)
	}

	podIP, err := parseCanonicalPodIPv4CIDR(podEndpoint.Spec.PodIpAddress)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidPodEndpoint, err)
	}
	if len(pod.Status.PodIPs) > 0 {
		for _, assignedIP := range pod.Status.PodIPs {
			if assignedIP.IP == podIP.String() {
				return nil
			}
		}
		return fmt.Errorf(
			"%w: endpoint IP %s is not assigned to Pod %s/%s",
			errInvalidPodEndpoint,
			podIP,
			pod.Namespace,
			pod.Name,
		)
	}
	if pod.Status.PodIP != "" && pod.Status.PodIP != podIP.String() {
		return fmt.Errorf(
			"%w: endpoint IP %s does not match Pod IP %s",
			errInvalidPodEndpoint,
			podIP,
			pod.Status.PodIP,
		)
	}
	return nil
}

// validateUniquePodIPClaim prevents two valid PodEndpoints from claiming the same address.
// The kernel route check remains the final atomic ownership boundary for concurrent changes.
func (r *PodEndpointReconciler) validateUniquePodIPClaim(
	ctx context.Context,
	podEndpoint *egressgatewayv1alpha1.PodEndpoint,
) error {
	claimedIP, err := parseCanonicalPodIPv4CIDR(podEndpoint.Spec.PodIpAddress)
	if err != nil {
		return fmt.Errorf("%w: %v", errInvalidPodEndpoint, err)
	}

	podEndpoints := &egressgatewayv1alpha1.PodEndpointList{}
	if err := r.List(ctx, podEndpoints); err != nil {
		return fmt.Errorf("failed to list PodEndpoints while validating IP ownership: %w", err)
	}
	for i := range podEndpoints.Items {
		other := &podEndpoints.Items[i]
		if other.Namespace == podEndpoint.Namespace && other.Name == podEndpoint.Name {
			continue
		}
		if err := r.validatePodEndpoint(ctx, other); err != nil {
			if errors.Is(err, errInvalidPodEndpoint) {
				continue
			}
			return fmt.Errorf("failed to validate PodEndpoint %s/%s while checking IP ownership: %w", other.Namespace, other.Name, err)
		}
		otherIP, err := parseCanonicalPodIPv4CIDR(other.Spec.PodIpAddress)
		if err != nil {
			return fmt.Errorf("failed to parse validated PodEndpoint IP %s/%s: %w", other.Namespace, other.Name, err)
		}
		if claimedIP.Equal(otherIP) {
			return fmt.Errorf(
				"%w: IP %s is already claimed by PodEndpoint %s/%s",
				errInvalidPodEndpoint,
				claimedIP,
				other.Namespace,
				other.Name,
			)
		}
	}
	return nil
}

func parseCanonicalPodIPv4CIDR(value string) (net.IP, error) {
	ip, ipNet, err := net.ParseCIDR(value)
	if err != nil {
		return nil, fmt.Errorf("pod IP address %q is not a valid CIDR: %w", value, err)
	}
	ipv4 := ip.To4()
	ones, bits := ipNet.Mask.Size()
	if ipv4 == nil || bits != net.IPv4len*8 || ones != bits {
		return nil, fmt.Errorf("pod IP address %q must be an IPv4 /32", value)
	}
	canonical := ipv4.String() + "/32"
	if value != canonical {
		return nil, fmt.Errorf("pod IP address %q must use canonical form %q", value, canonical)
	}
	return ipv4, nil
}

// updateGatewayNodeStatus updates the GatewayStatus ReadyPeerConfigurations list based on the provided peerConfigs.
// When op is PeerUpdateOpAdd, the peerConfigs will be added to the existing ready peers list.
// When op is PeerUpdateOpDelete, all peers currently on the GatewayStatus will be removed except for those in peerConfigs,
// effectively treating peerConfigs as the expected set of peers to keep.
func (r *PodEndpointReconciler) updateGatewayNodeStatus(
	ctx context.Context,
	peerConfigs []egressgatewayv1alpha1.PeerConfiguration,
	op PeerUpdateOperation,
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
			if op == PeerUpdateOpDelete {
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
		peerMap := make(map[string]*egressgatewayv1alpha1.PeerConfiguration)
		for _, peerConfig := range gwStatus.Spec.ReadyPeerConfigurations {
			peerConfig := peerConfig
			peerMap[peerConfig.PublicKey] = &peerConfig
		}

		changed := false
		switch op {
		case PeerUpdateOpAdd:
			for i, peerConfig := range peerConfigs {
				if _, exists := peerMap[peerConfig.PublicKey]; !exists {
					peerMap[peerConfig.PublicKey] = &peerConfigs[i]
					changed = true
				}
			}
		case PeerUpdateOpDelete:
			peersToKeep := make(map[string]bool)
			for _, peerConfig := range peerConfigs {
				peersToKeep[peerConfig.PublicKey] = true
			}

			for pk := range peerMap {
				if !peersToKeep[pk] {
					delete(peerMap, pk)
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
