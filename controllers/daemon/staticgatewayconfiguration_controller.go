// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/compat"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway node network according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	CompatClient  *compat.CompatClient // Added for Go 1.25.0 compatibility
	TickerEvents  chan event.GenericEvent
	LBProbeServer *healthprobe.LBProbeServer
	Netlink       netlinkwrapper.Interface
	NetNS         netnswrapper.Interface
	IPTables      utiliptables.Interface
	WgCtrl        wgctrlwrapper.Interface
}

//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations/status,verbs=get
//+kubebuilder:rbac:groups=core,namespace=kube-egress-gateway-system,resources=secrets,verbs=get;list;watch
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

var (
	nodeMeta *imds.InstanceMetadata
	lbMeta   *imds.LoadBalancerMetadata
	nodeTags map[string]string
)

func InitNodeMetadata() error {
	var err error
	nodeMeta, err = imds.GetInstanceMetadata()
	if err != nil {
		return err
	}
	lbMeta, err = imds.GetLoadBalancerMetadata()
	if err != nil {
		return err
	}
	if nodeMeta == nil || lbMeta == nil {
		return fmt.Errorf("failed to setup controller: nodeMeta or lbMeta is nil")
	}
	nodeTags = parseNodeTags()
	return nil
}

func (r *StaticGatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Got an event from cleanup ticker
	if req.NamespacedName.Namespace == "" && req.NamespacedName.Name == "" {
		if err := r.cleanUp(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clean up orphaned network configurations: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Fetch the StaticGatewayConfiguration instance.
	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.CompatClient.Get(ctx, req.NamespacedName, gwConfig); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch StaticGatewayConfiguration instance")
		return ctrl.Result{}, err
	}

	if !isReady(gwConfig) {
		// gateway setup hasn't completed yet
		return ctrl.Result{}, nil
	}

	if !applyToNode(gwConfig) {
		// gwConfig does not apply to this node
		return ctrl.Result{}, nil
	}

	if !gwConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		if err := r.cleanUp(ctx); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to clean up deleted StaticGatewayConfiguration %s/%s: %w", gwConfig.Namespace, gwConfig.Name, err)
		}
		return ctrl.Result{}, nil
	}

	// Reconcile gateway configuration
	return ctrl.Result{}, r.reconcile(ctx, gwConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Netlink = netlinkwrapper.NewNetLink()
	r.NetNS = netnswrapper.NewNetNS()
	r.IPTables = utiliptables.New(utiliptables.ProtocolIPv4)
	r.WgCtrl = wgctrlwrapper.NewWgCtrl()
	r.CompatClient = compat.NewCompatClient(r.Client)
	controller, err := ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.StaticGatewayConfiguration{}).
		// We need to watch GatewayVMConfiguration also, because vmSecondaryIP may change, e.g. duing upgrade
		// we can use EnqueueRequestForObject because GatewayVMConfiguration has the same namespace/name as StaticGatewayConfiguration
		Watches(&egressgatewayv1alpha1.GatewayVMConfiguration{}, &handler.EnqueueRequestForObject{}).
		Build(r)
	if err != nil {
		return err
	}
	return controller.Watch(source.Channel(r.TickerEvents, &handler.EnqueueRequestForObject{}))
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) error {
	log := log.FromContext(ctx)
	log.Info("Reconciling gateway configuration")

	// get wireguard private key from secret
	privateKey, err := r.getWireguardPrivateKey(ctx, gwConfig)
	if err != nil {
		return err
	}

	// add lb ip (if not exists) to eth0
	if err := r.reconcileIlbIPOnHost(ctx, gwConfig.Status.GatewayServerProfile.Ip); err != nil {
		return err
	}

	// remove secondary ip from eth0
	vmPrimaryIP, vmSecondaryIP, err := r.getVMIP(ctx, gwConfig)
	if err != nil {
		return err
	}

	if err := r.removeSecondaryIpFromHost(ctx, vmSecondaryIP); err != nil {
		return err
	}

	// avoid masquerading packets from gateway namespace, as they're already sNATed
	if err := r.ensureIPTablesChain(
		ctx,
		utiliptables.TableNAT,
		utiliptables.Chain("EGRESS-GATEWAY-SNAT"), // target chain
		utiliptables.ChainPostrouting,             // source chain
		"kube-egress-gateway no MASQUERADE",
		nil); err != nil {
		return err
	}

	if err := r.ensureIPTablesChain(
		ctx,
		utiliptables.TableNAT,
		utiliptables.Chain(fmt.Sprintf("EGRESS-%s", strings.ReplaceAll(vmSecondaryIP, ".", "-"))), // target chain
		utiliptables.Chain("EGRESS-GATEWAY-SNAT"),                                                 // source chain
		fmt.Sprintf("kube-egress-gateway no sNAT packet from ip %s", vmSecondaryIP),
		[][]string{
			{"-s", vmSecondaryIP + "/32", "-j", "ACCEPT"},
		}); err != nil {
		return err
	}

	// configure gateway namespace (if not exists)
	if err := r.configureGatewayNamespace(ctx, gwConfig, privateKey, vmPrimaryIP, vmSecondaryIP); err != nil {
		return err
	}

	// update gateway status
	gwStatus := egressgatewayv1alpha1.GatewayConfiguration{
		StaticGatewayConfiguration: fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name),
		InterfaceName:              getWireguardInterfaceName(gwConfig),
	}
	if err := r.updateGatewayNodeStatus(ctx, gwStatus, true /* add */); err != nil {
		return err
	}

	if err := r.LBProbeServer.AddGateway(string(gwConfig.GetUID())); err != nil {
		return err
	}

	log.Info("Gateway configuration reconciled")
	return nil
}

func (r *StaticGatewayConfigurationReconciler) cleanUp(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Cleaning up orphaned gateway network configurations")

	gwConfigList := &egressgatewayv1alpha1.StaticGatewayConfigurationList{}
	if err := r.CompatClient.List(ctx, gwConfigList); err != nil {
		return fmt.Errorf("failed to list staticGatewayConfigurations: %w", err)
	}
	existingWgLinks := make(map[string]struct{})
	existingIPs := make(map[string]struct{})
	hasActiveGateway := false
	for _, gwConfig := range gwConfigList.Items {
		if applyToNode(&gwConfig) && gwConfig.DeletionTimestamp.IsZero() {
			_, vmSecondaryIP, err := r.getVMIP(ctx, &gwConfig)
			if err != nil {
				log.Error(err, "failed to get VM secondaryIP during cleanup", "gwConfig", fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name))
				continue
			}
			existingWgLinks[getWireguardInterfaceName(&gwConfig)] = struct{}{}
			existingIPs[vmSecondaryIP] = struct{}{}
			hasActiveGateway = true
		}
	}

	gwns, err := r.NetNS.GetNS(consts.GatewayNetnsName)
	if err != nil {
		return fmt.Errorf("failed to get network namespace %s: %w", consts.GatewayNetnsName, err)
	}
	defer func() { _ = gwns.Close() }()

	var links []netlink.Link
	var ips []netlink.Addr
	if err := gwns.Do(func(nn ns.NetNS) error {
		var err error
		links, err = r.Netlink.LinkList()
		if err != nil {
			return fmt.Errorf("failed to list links in gateway namespace: %w", err)
		}
		hostLink, err := r.Netlink.LinkByName(consts.HostLinkName)
		if err != nil {
			return fmt.Errorf("failed to get host link in gateway namespace: %w", err)
		}
		ips, err = r.Netlink.AddrList(hostLink, nl.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("failed to list addresses on host0 in gateway namespace: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	for _, ip := range ips {
		if _, ok := existingIPs[ip.IP.String()]; !ok {
			log.Info("Removing orphaned IP", "ip", ip.IP.String())
			if err := r.ensureDeleteIP(ctx, gwns, ip); err != nil {
				log.Error(err, fmt.Sprintf("failed to cleanup vmSecondaryIP %s", ip.IP.String()))
			}
		}
	}

	for _, link := range links {
		if strings.HasPrefix(link.Attrs().Name, consts.WiregaurdLinkNamePrefix) {
			if _, ok := existingWgLinks[link.Attrs().Name]; !ok {
				log.Info("Removing orphaned wireguard link", "link", link.Attrs().Name)
				if err := r.ensureDeleteLink(ctx, gwns, link); err != nil {
					log.Error(err, fmt.Sprintf("failed to cleanup wireguard link %s", link.Attrs().Name))
				}
			}
		}
	}

	if !hasActiveGateway {
		log.Info("No active gateway found, cleaning up leftover network configurations")
		if err := r.reconcileIlbIPOnHost(ctx, ""); err != nil {
			return fmt.Errorf("failed to cleanup ILB IP on host: %w", err)
		}

		if err := r.removeIPTablesChains(
			ctx,
			utiliptables.TableNAT,
			[]utiliptables.Chain{utiliptables.Chain("EGRESS-GATEWAY-SNAT")},
			[]utiliptables.Chain{utiliptables.ChainPostrouting},
			[]string{"kube-egress-gateway no MASQUERADE"},
		); err != nil {
			return fmt.Errorf("failed to delete iptables chain EGRESS-GATEWAY-SNAT: %w", err)
		}
	}

	log.Info("Network namespace cleanup completed")
	return nil
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleteLink(ctx context.Context, gwns ns.NetNS, link netlink.Link) error {
	log := log.FromContext(ctx)

	linkName := link.Attrs().Name
	if err := gwns.Do(func(nn ns.NetNS) error {
		log.Info("Deleting link", "link", link.Attrs().Name)
		err := r.Netlink.LinkDel(link)
		if err != nil {
			return fmt.Errorf("failed to delete link %s: %w", linkName, err)
		}

		mark, err := getPacketMark(linkName)
		if err != nil {
			return err
		}
		log.Info("Removing iptables rules", "mark", mark)
		if err := r.removeIPTablesChains(
			ctx,
			utiliptables.TableNAT,
			[]utiliptables.Chain{
				utiliptables.Chain(fmt.Sprintf("EGRESS-GATEWAY-MARK-%d", mark)),
				utiliptables.Chain(fmt.Sprintf("EGRESS-GATEWAY-SNAT-%d", mark)),
			}, // target chain
			[]utiliptables.Chain{
				utiliptables.ChainPrerouting,
				utiliptables.ChainPostrouting,
			}, // source chain
			[]string{
				fmt.Sprintf("kube-egress-gateway mark packets from gateway link %s", linkName),
				fmt.Sprintf("kube-egress-gateway sNAT packets from gateway link %s", linkName),
			},
		); err != nil {
			return fmt.Errorf("failed to cleanup iptables rules for link %s and mark %d: %w", linkName, mark, err)
		}
		return nil
	}); err != nil {
		return err
	}

	// update gateway status
	gwStatus := egressgatewayv1alpha1.GatewayConfiguration{
		InterfaceName: link.Attrs().Name,
	}
	if err := r.updateGatewayNodeStatus(ctx, gwStatus, false /* add */); err != nil {
		return err
	}

	if err := r.LBProbeServer.RemoveGateway(link.Attrs().Alias); err != nil {
		return err
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleteIP(ctx context.Context, gwns ns.NetNS, ip netlink.Addr) error {
	log := log.FromContext(ctx)
	if err := gwns.Do(func(nn ns.NetNS) error {
		log.Info("Deleting IP from host0", "ip", ip.IP.String())
		hostLink, err := r.Netlink.LinkByName(consts.HostLinkName)
		if err != nil {
			return fmt.Errorf("failed to get host link in gateway namespace: %w", err)
		}
		if err := r.Netlink.AddrDel(hostLink, &ip); err != nil {
			return fmt.Errorf("failed to delete IP %s: %w", ip.IP.String(), err)
		}
		return nil
	}); err != nil {
		return err
	}

	routes, err := r.Netlink.RouteList(nil, nl.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list routes in host namespace: %w", err)
	}
	for _, route := range routes {
		route := route
		if route.Dst != nil && route.Dst.IP.Equal(ip.IP) {
			log.Info("Deleting route in host namespace to vmSecondaryIP", "route", route)
			if err := r.Netlink.RouteDel(&route); err != nil {
				return fmt.Errorf("failed to delete route to %s: %w", ip.IP.String(), err)
			}
		}
	}

	log.Info("Deleting no-sNAT rule for vmSecondaryIP", "ip", ip.IP.String())
	if err := r.removeIPTablesChains(
		ctx,
		utiliptables.TableNAT,
		[]utiliptables.Chain{utiliptables.Chain(fmt.Sprintf("EGRESS-%s", strings.ReplaceAll(ip.IP.String(), ".", "-")))}, // target chain
		[]utiliptables.Chain{utiliptables.Chain("EGRESS-GATEWAY-SNAT")},                                                  // source chain
		[]string{fmt.Sprintf("kube-egress-gateway no sNAT packet from ip %s", ip.IP.String())},
	); err != nil {
		return fmt.Errorf("failed to clean up no-sNAT rule for vmSecondaryIP %s: %w", ip.IP.String(), err)
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) getWireguardPrivateKey(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) (*wgtypes.Key, error) {
	secretKey := &types.NamespacedName{
		Namespace: gwConfig.Status.PrivateKeySecretRef.Namespace,
		Name:      gwConfig.Status.PrivateKeySecretRef.Name,
	}
	secret := &corev1.Secret{}
	if err := r.CompatClient.Get(ctx, *secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to retrieve wireguard private key secret: %w", err)
	}

	wgPrivateKeyByte, ok := secret.Data[consts.WireguardPrivateKeyName]
	if !ok {
		return nil, fmt.Errorf("failed to retrieve private key from secret %s/%s", secretKey.Namespace, secretKey.Name)
	}
	wgPrivateKey, err := wgtypes.ParseKey(string(wgPrivateKeyByte))
	if err != nil {
		return nil, err
	}
	return &wgPrivateKey, nil
}

func (r *StaticGatewayConfigurationReconciler) getVMIP(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) (string, string, error) {
	log := log.FromContext(ctx)

	nodeName := nodeMeta.Compute.OSProfile.ComputerName
	var primaryIP, secondaryIP string

	// Fetch the StaticGatewayConfiguration instance.
	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{}
	if err := r.CompatClient.Get(ctx, types.NamespacedName{Namespace: gwConfig.Namespace, Name: gwConfig.Name}, vmConfig); err != nil {
		return "", "", err
	}

	// this can happen in cleanup process when vmConfig is not ready yet
	if vmConfig.Status == nil {
		return "", "", fmt.Errorf("status is nil for GatewayVMConfiguration %s/%s", vmConfig.Namespace, vmConfig.Name)
	}

	for _, vmProfile := range vmConfig.Status.GatewayVMProfiles {
		if vmProfile.NodeName == nodeName {
			primaryIP = vmProfile.PrimaryIP
			secondaryIP = vmProfile.SecondaryIP
			break
		}
	}

	if primaryIP == "" || secondaryIP == "" {
		return "", "", fmt.Errorf("failed to find primary or secondary IP for node %s", nodeName)
	}

	log.Info("Found primary and secondary IP for node", "nodeName", nodeName, "primaryIP", primaryIP, "secondaryIP", secondaryIP)

	return primaryIP, secondaryIP, nil
}

func isReady(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	wgProfile := gwConfig.Status.GatewayServerProfile
	return gwConfig.Status.EgressIpPrefix != "" && wgProfile.Ip != "" &&
		wgProfile.Port != 0 && wgProfile.PublicKey != "" &&
		wgProfile.PrivateKeySecretRef != nil
}

func applyToNode(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	if gwConfig.Spec.GatewayNodepoolName != "" {
		name, ok := nodeTags[consts.AKSNodepoolTagKey]
		return ok && strings.EqualFold(name, gwConfig.Spec.GatewayNodepoolName)
	} else {
		vmssProfile := gwConfig.Spec.GatewayVmssProfile
		return strings.EqualFold(vmssProfile.VmssName, nodeMeta.Compute.VMScaleSetName) &&
			strings.EqualFold(vmssProfile.VmssResourceGroup, nodeMeta.Compute.ResourceGroupName)
	}
}

func parseNodeTags() map[string]string {
	tags := make(map[string]string)
	tagStrs := strings.Split(nodeMeta.Compute.Tags, ";")
	for _, tag := range tagStrs {
		kv := strings.Split(tag, ":")
		if len(kv) == 2 {
			tags[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	return tags
}

func (r *StaticGatewayConfigurationReconciler) reconcileIlbIPOnHost(ctx context.Context, ilbIP string) error {
	log := log.FromContext(ctx)
	eth0, err := r.Netlink.LinkByName("eth0")
	if err != nil {
		return fmt.Errorf("failed to retrieve link eth0: %w", err)
	}

	if len(nodeMeta.Network.Interface) == 0 || len(nodeMeta.Network.Interface[0].IPv4.Subnet) == 0 {
		return fmt.Errorf("imds does not provide subnet information about the node")
	}
	prefix, err := strconv.Atoi(nodeMeta.Network.Interface[0].IPv4.Subnet[0].Prefix)
	if err != nil {
		return fmt.Errorf("failed to retrieve and parse prefix: %w", err)
	}

	addresses, err := r.Netlink.AddrList(eth0, nl.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to retrieve IP addresses for eth0: %w", err)
	}

	if ilbIP == "" {
		// cleanup process
		for _, address := range addresses {
			if address.Label == consts.ILBIPLabel {
				log.Info("Removing ILB IP from eth0", "ilb IP", address.IPNet.String())
				if err := r.Netlink.AddrDel(eth0, &address); err != nil {
					return fmt.Errorf("failed to delete ILB IP from eth0: %w", err)
				}
			}
		}
		return nil
	}

	ilbIpCidr := fmt.Sprintf("%s/%d", ilbIP, prefix)
	ilbIpNet, err := netlink.ParseIPNet(ilbIpCidr)
	if err != nil {
		return fmt.Errorf("failed to parse ILB IP address: %s", ilbIpCidr)
	}

	addressPresent := false
	for _, address := range addresses {
		if address.IPNet.IP.Equal(ilbIpNet.IP) {
			addressPresent = true
			break
		}
	}

	if !addressPresent {
		log.Info("Adding ILB IP to eth0", "ilb IP", ilbIpCidr)
		if err := r.Netlink.AddrAdd(eth0, &netlink.Addr{
			IPNet: ilbIpNet,
			Label: consts.ILBIPLabel,
		}); err != nil {
			return fmt.Errorf("failed to add ILB IP to eth0: %w", err)
		}
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) removeSecondaryIpFromHost(ctx context.Context, ip string) error {
	log := log.FromContext(ctx)
	eth0, err := r.Netlink.LinkByName("eth0")
	if err != nil {
		return fmt.Errorf("failed to retrieve link eth0: %w", err)
	}

	addresses, err := r.Netlink.AddrList(eth0, nl.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to retrieve IP addresses for eth0: %w", err)
	}

	for _, address := range addresses {
		if address.IP.String() == ip {
			log.Info("Removing secondary IP from eth0", "secondary_ip", address.IP.String())
			if err := r.Netlink.AddrDel(eth0, &address); err != nil {
				return fmt.Errorf("failed to remove secondary ip from eth0: %w", err)
			}
		}
	}

	return nil
}

func (r *StaticGatewayConfigurationReconciler) configureGatewayNamespace(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	privateKey *wgtypes.Key,
	vmPrimaryIP string,
	vmSecondaryIP string,
) error {
	gwns, err := r.NetNS.GetNS(consts.GatewayNetnsName)
	if err != nil {
		return fmt.Errorf("failed to get network namespace %s: %w", consts.GatewayNetnsName, err)
	}
	defer func() { _ = gwns.Close() }()

	if err := r.reconcileWireguardLink(ctx, gwns, gwConfig, privateKey); err != nil {
		return err
	}

	if err := r.reconcileVethPair(ctx, gwns, vmPrimaryIP, vmSecondaryIP); err != nil {
		return err
	}

	return gwns.Do(func(nn ns.NetNS) error {
		looplink, err := r.Netlink.LinkByName("lo")
		if err != nil {
			return fmt.Errorf("failed to retrieve link lo: %w", err)
		}
		if err := r.Netlink.LinkSetUp(looplink); err != nil {
			return fmt.Errorf("failed to set lo up: %w", err)
		}

		linkName := getWireguardInterfaceName(gwConfig)
		mark, err := getPacketMark(linkName)
		if err != nil {
			return err
		}
		if err := r.ensureIPTablesChain(
			ctx,
			utiliptables.TableNAT,
			utiliptables.Chain(fmt.Sprintf("EGRESS-GATEWAY-MARK-%d", mark)), // target chain
			utiliptables.ChainPrerouting,                                    // source chain
			fmt.Sprintf("kube-egress-gateway mark packets from gateway link %s", linkName),
			[][]string{
				{"-i", linkName, "-j", "CONNMARK", "--set-mark", fmt.Sprintf("%d", mark)},
			}); err != nil {
			return err
		}

		if err := r.ensureIPTablesChain(
			ctx,
			utiliptables.TableNAT,
			utiliptables.Chain(fmt.Sprintf("EGRESS-GATEWAY-SNAT-%d", mark)), // target chain
			utiliptables.ChainPostrouting,                                   // source chain
			fmt.Sprintf("kube-egress-gateway sNAT packets from gateway link %s", linkName),
			[][]string{
				{"-o", consts.HostLinkName, "-m", "connmark", "--mark", fmt.Sprintf("%d", mark), "-j", "SNAT", "--to-source", vmSecondaryIP},
			}); err != nil {
			return err
		}

		return nil
	})
}

func (r *StaticGatewayConfigurationReconciler) reconcileWireguardLink(
	ctx context.Context,
	gwns ns.NetNS,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	privateKey *wgtypes.Key,
) error {
	log := log.FromContext(ctx)
	linkName := getWireguardInterfaceName(gwConfig)
	var wgLink netlink.Link
	var err error
	if err = gwns.Do(func(nn ns.NetNS) error {
		wgLink, err = r.Netlink.LinkByName(linkName)
		if err != nil {
			if _, ok := err.(netlink.LinkNotFoundError); !ok {
				return fmt.Errorf("failed to get wireguard link in gateway namespace: %w", err)
			}
			wgLink = nil
		}
		return nil
	}); err != nil {
		return err
	}

	if wgLink == nil {
		log.Info("Creating wireguard link")
		if err := r.createWireguardLink(gwns, linkName, string(gwConfig.GetUID())); err != nil {
			return fmt.Errorf("failed to create wireguard link: %w", err)
		}
	}

	return gwns.Do(func(nn ns.NetNS) error {
		wgLink, err := r.Netlink.LinkByName(linkName)
		if err != nil {
			return fmt.Errorf("failed to get wireguard link in gateway namespace after creation: %w", err)
		}
		gwIP, _ := netlink.ParseIPNet(consts.GatewayIP)
		gwLinkAddr := netlink.Addr{
			IPNet: gwIP,
		}

		wgLinkAddrs, err := r.Netlink.AddrList(wgLink, nl.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("failed to retrieve address list from wireguard link: %w", err)
		}

		foundLink := false
		for _, addr := range wgLinkAddrs {
			if addr.Equal(gwLinkAddr) {
				log.Info("Found wireguard link address")
				foundLink = true
				break
			}
		}

		if !foundLink {
			log.Info("Adding wireguard link address")
			err = r.Netlink.AddrAdd(wgLink, &gwLinkAddr)
			if err != nil {
				return fmt.Errorf("failed to add wireguard link address: %w", err)
			}
		}

		err = r.Netlink.LinkSetUp(wgLink)
		if err != nil {
			return fmt.Errorf("failed to set wireguard link up: %w", err)
		}

		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		wgConfig := wgtypes.Config{
			ListenPort: to.Ptr(int(gwConfig.Status.Port)),
			PrivateKey: privateKey,
		}

		device, err := wgClient.Device(linkName)
		if err != nil {
			return fmt.Errorf("failed to get wireguard link configuration: %w", err)
		}

		if device.PrivateKey.String() != wgConfig.PrivateKey.String() || device.ListenPort != to.Val(wgConfig.ListenPort) {
			log.Info("Updating wireguard link config", "orig port", device.ListenPort, "cur port", to.Val(wgConfig.ListenPort),
				"private key difference", device.PrivateKey.String() != wgConfig.PrivateKey.String())
			err = wgClient.ConfigureDevice(linkName, wgConfig)
			if err != nil {
				return fmt.Errorf("failed to add peer to wireguard link: %w", err)
			}
		}
		return nil
	})
}

func (r *StaticGatewayConfigurationReconciler) createWireguardLink(gwns ns.NetNS, linkName, linkAlias string) error {
	succeed := false
	attr := netlink.NewLinkAttrs()
	attr.Name = linkName
	attr.Alias = linkAlias
	wg := &netlink.Wireguard{LinkAttrs: attr}
	err := r.Netlink.LinkAdd(wg)
	if err != nil {
		return fmt.Errorf("failed to create wireguard link: %w", err)
	}
	defer func() {
		if !succeed {
			_ = r.Netlink.LinkDel(wg)
		}
	}()

	wgLink, err := r.Netlink.LinkByName(linkName)
	if err != nil {
		return fmt.Errorf("failed to get wireguard link in host namespace: %w", err)
	}

	if err := r.Netlink.LinkSetNsFd(wgLink, int(gwns.Fd())); err != nil {
		return fmt.Errorf("failed to move wireguard link to gateway namespace: %w", err)
	}

	succeed = true
	return nil
}

func (r *StaticGatewayConfigurationReconciler) reconcileVethPair(
	ctx context.Context,
	gwns ns.NetNS,
	vmPrimaryIP string,
	vmSecondaryIP string,
) error {
	log := log.FromContext(ctx)
	if err := r.reconcileVethPairInHost(ctx, gwns, vmSecondaryIP); err != nil {
		return fmt.Errorf("failed to reconcile veth pair in host namespace: %w", err)
	}

	return gwns.Do(func(nn ns.NetNS) error {
		hostLink, err := r.Netlink.LinkByName(consts.HostLinkName)
		if err != nil {
			return fmt.Errorf("failed to get host link in gateway namespace: %w", err)
		}

		_, snatIPNet, err := net.ParseCIDR(vmSecondaryIP + "/32")
		if err != nil {
			return fmt.Errorf("failed to parse SNAT IP(%s) for host interface: %w", vmSecondaryIP+"/32", err)
		}
		hostLinkAddr := netlink.Addr{IPNet: snatIPNet}

		hostLinkAddrs, err := r.Netlink.AddrList(hostLink, nl.FAMILY_ALL)
		if err != nil {
			return fmt.Errorf("failed to retrieve address list from wireguard link: %w", err)
		}

		foundLink := false
		for _, addr := range hostLinkAddrs {
			if addr.Equal(hostLinkAddr) {
				log.Info("Found host link address in gateway namespace")
				foundLink = true
				break
			}
		}

		if !foundLink {
			log.Info("Adding host link address in gateway namespace")
			err = r.Netlink.AddrAdd(hostLink, &hostLinkAddr)
			if err != nil {
				return fmt.Errorf("failed to add host link address in gateway namespace: %w", err)
			}
		}

		err = r.Netlink.LinkSetUp(hostLink)
		if err != nil {
			return fmt.Errorf("failed to set host link up: %w", err)
		}

		_, vmSnatCidr, err := net.ParseCIDR(vmPrimaryIP + "/32")
		if err != nil {
			return fmt.Errorf("failed to parse CIDR %s/32: %w", vmPrimaryIP+"/32", err)
		}

		err = r.addOrReplaceRoute(ctx, &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Scope:     netlink.SCOPE_LINK,
			Dst:       vmSnatCidr,
		})
		if err != nil {
			return fmt.Errorf("failed to create route to VM primary IP %s via gateway interface: %w", vmPrimaryIP, err)
		}

		err = r.addOrReplaceRoute(ctx, &netlink.Route{
			LinkIndex: hostLink.Attrs().Index,
			Scope:     netlink.SCOPE_UNIVERSE,
			Dst:       nil,
			Gw:        net.ParseIP(vmPrimaryIP),
		})
		if err != nil {
			return fmt.Errorf("failed to create default route via %s: %w", vmPrimaryIP, err)
		}
		return nil
	})
}

func (r *StaticGatewayConfigurationReconciler) reconcileVethPairInHost(
	ctx context.Context,
	gwns ns.NetNS,
	snatIP string,
) error {
	log := log.FromContext(ctx)
	succeed := false

	la := netlink.NewLinkAttrs()
	la.Name = consts.HostVethLinkName

	mainLink, err := r.Netlink.LinkByName(la.Name)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		log.Info("Creating veth pair in host namespace")
		veth := &netlink.Veth{
			LinkAttrs: la,
			PeerName:  consts.HostLinkName,
		}
		err := r.Netlink.LinkAdd(veth)
		if err != nil {
			return fmt.Errorf("failed to add veth pair: %w", err)
		}
		defer func() {
			if !succeed {
				_ = r.Netlink.LinkDel(veth)
			}
		}()
		mainLink, err = r.Netlink.LinkByName(la.Name)
		if err != nil {
			return fmt.Errorf("failed to get veth link in host namespace after creation: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get veth link in host namespace: %w", err)
	}

	err = r.Netlink.LinkSetUp(mainLink)
	if err != nil {
		return fmt.Errorf("failed to set veth link in host namespace up: %w", err)
	}

	_, snatIPNet, err := net.ParseCIDR(snatIP + "/32")
	if err != nil {
		return fmt.Errorf("failed to parse SNAT IP %s: %w", snatIP+"/32", err)
	}
	route := &netlink.Route{
		LinkIndex: mainLink.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Dst:       snatIPNet,
	}
	if err = r.addOrReplaceRoute(ctx, route); err != nil {
		return fmt.Errorf("failed to create route to SNAT IP %s via gateway interface: %w", snatIP, err)
	}
	defer func() {
		if !succeed {
			_ = r.Netlink.RouteDel(route)
		}
	}()

	hostLink, err := r.Netlink.LinkByName(consts.HostLinkName)
	if err == nil {
		if err := r.Netlink.LinkSetNsFd(hostLink, int(gwns.Fd())); err != nil {
			return fmt.Errorf("failed to move veth peer link to gateway namespace: %w", err)
		}
	} else if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return fmt.Errorf("failed to get veth peer link in host namespace: %w", err)
	}

	succeed = true
	return nil
}

func (r *StaticGatewayConfigurationReconciler) addOrReplaceRoute(ctx context.Context, route *netlink.Route) error {
	log := log.FromContext(ctx)
	equalRoute := func(r1, r2 *netlink.Route) bool {
		size1, _ := to.Val(r1.Dst).Mask.Size()
		size2, _ := to.Val(r2.Dst).Mask.Size()
		return r1.LinkIndex == r2.LinkIndex && r1.Scope == r2.Scope &&
			to.Val(r1.Dst).IP.Equal(to.Val(r2.Dst).IP) && size1 == size2 && r1.Gw.Equal(r2.Gw)
	}
	routes, err := r.Netlink.RouteList(nil, nl.FAMILY_ALL)
	if err != nil {
		return err
	}
	foundRoute := false
	for i := range routes {
		if equalRoute(&routes[i], route) {
			foundRoute = true
		}
	}
	if !foundRoute {
		log.Info("Adding new route", "route", *route)
		if err = r.Netlink.RouteReplace(route); err != nil {
			return err
		}
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) ensureIPTablesChain(
	ctx context.Context,
	table utiliptables.Table,
	targetChain utiliptables.Chain,
	sourceChain utiliptables.Chain,
	jumpRuleComment string,
	chainRules [][]string,
) error {
	log := log.FromContext(ctx)

	// ensure target chain exists
	log.Info("Ensuring iptables chain", "table", table, "target chain", targetChain)
	if _, err := r.IPTables.EnsureChain(table, targetChain); err != nil {
		return fmt.Errorf("failed to ensure chain %s in table %s: %w", targetChain, table, err)
	}

	// ensure jump rule exists, we use EnsureRule because we do not want to flush all rules in the source chain
	log.Info("Ensuring jump rule", "source chain", sourceChain)
	if _, err := r.IPTables.EnsureRule(utiliptables.Prepend, table, sourceChain, "-m", "comment", "--comment", jumpRuleComment, "-j", string(targetChain)); err != nil {
		return fmt.Errorf("failed to ensure jump rule from chain %s to chain %s in table %s: %w", sourceChain, targetChain, table, err)
	}

	if len(chainRules) == 0 {
		return nil
	}

	// ensure all rules in the target chain atomically
	lines := bytes.NewBuffer(nil)
	writeLine(lines, "*"+string(table))
	writeLine(lines, utiliptables.MakeChainLine(targetChain))
	for _, rule := range chainRules {
		writeRule(lines, string(utiliptables.Append), targetChain, rule...)
	}
	writeLine(lines, "COMMIT")
	log.Info("Restoring rules", "rules", lines.String())
	if err := r.IPTables.RestoreAll(lines.Bytes(), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters); err != nil {
		return fmt.Errorf("failed to restore rules in chain %s in table %s: %w", targetChain, table, err)
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) removeIPTablesChains(
	ctx context.Context,
	table utiliptables.Table,
	targetChains []utiliptables.Chain,
	sourceChains []utiliptables.Chain,
	jumpRuleComments []string,
) error {
	log := log.FromContext(ctx)

	iptablesData := bytes.NewBuffer(nil)
	if err := r.IPTables.SaveInto(table, iptablesData); err != nil {
		return fmt.Errorf("failed to save iptables data for table %s: %w", table, err)
	}

	existingChains := utiliptables.GetChainsFromTable(iptablesData.Bytes())
	for i, targetChain := range targetChains {
		sourceChain := sourceChains[i]
		jumpRuleComment := jumpRuleComments[i]
		if _, ok := existingChains[targetChain]; ok {
			// delete jump rule first
			log.Info("Deleting jump rule", "source chain", sourceChain, "target chain", targetChain)
			if err := r.IPTables.DeleteRule(table, sourceChain, "-m", "comment", "--comment", jumpRuleComment, "-j", string(targetChain)); err != nil {
				return fmt.Errorf("failed to delete jump rule from chain %s to chain %s in table %s: %w", sourceChain, targetChain, table, err)
			}

			log.Info("Flushing and deleting chain", "table", table, "target chain", targetChain)
			lines := bytes.NewBuffer(nil)
			writeLine(lines, "*"+string(table))
			writeLine(lines, utiliptables.MakeChainLine(targetChain))
			writeLine(lines, "-X", string(targetChain))
			writeLine(lines, "COMMIT")
			if err := r.IPTables.Restore(table, lines.Bytes(), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters); err != nil {
				return fmt.Errorf("failed to restore iptables table %s: %w", table, err)
			}
		}
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) updateGatewayNodeStatus(
	ctx context.Context,
	gwConfig egressgatewayv1alpha1.GatewayConfiguration,
	add bool,
) error {
	log := log.FromContext(ctx)
	gwStatusKey := types.NamespacedName{
		Namespace: os.Getenv(consts.PodNamespaceEnvKey),
		Name:      os.Getenv(consts.NodeNameEnvKey),
	}

	gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
	if err := r.CompatClient.Get(ctx, gwStatusKey, gwStatus); err != nil {
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
			if err := r.CompatClient.Get(ctx, types.NamespacedName{Name: os.Getenv(consts.NodeNameEnvKey)}, node); err != nil {
				return fmt.Errorf("failed to get current node: %w", err)
			}

			gwStatus := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gwStatusKey.Name,
					Namespace: gwStatusKey.Namespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyGatewayConfigurations: []egressgatewayv1alpha1.GatewayConfiguration{gwConfig},
				},
			}
			if err := controllerutil.SetOwnerReference(node, gwStatus, r.Client.Scheme()); err != nil {
				return fmt.Errorf("failed to set gwStatus owner reference to node: %w", err)
			}
			log.Info("Creating new gateway status object")
			if err := r.CompatClient.Create(ctx, gwStatus); err != nil {
				return fmt.Errorf("failed to create gwStatus object: %w", err)
			}
		}
	} else {
		changed := false
		found := false
		for i, gwConf := range gwStatus.Spec.ReadyGatewayConfigurations {
			if gwConf.InterfaceName == gwConfig.InterfaceName {
				if !add {
					changed = true
					gwStatus.Spec.ReadyGatewayConfigurations = append(gwStatus.Spec.ReadyGatewayConfigurations[:i], gwStatus.Spec.ReadyGatewayConfigurations[i+1:]...)
				}
				found = true
				break
			}
		}
		if add && !found {
			gwStatus.Spec.ReadyGatewayConfigurations = append(gwStatus.Spec.ReadyGatewayConfigurations, gwConfig)
			changed = true
		}
		if !add {
			for i := len(gwStatus.Spec.ReadyPeerConfigurations) - 1; i >= 0; i = i - 1 {
				if gwStatus.Spec.ReadyPeerConfigurations[i].InterfaceName == gwConfig.InterfaceName {
					changed = true
					gwStatus.Spec.ReadyPeerConfigurations = append(gwStatus.Spec.ReadyPeerConfigurations[:i], gwStatus.Spec.ReadyPeerConfigurations[i+1:]...)
				}
			}
		}
		if changed {
			log.Info("Updating gateway status object")
			if err := r.CompatClient.Update(ctx, gwStatus); err != nil {
				return fmt.Errorf("failed to update gwStatus object: %w", err)
			}
		}
	}
	return nil
}

func getWireguardInterfaceName(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) string {
	return consts.WiregaurdLinkNamePrefix + fmt.Sprintf("%d", gwConfig.Status.Port)
}

func getPacketMark(linkName string) (int, error) {
	mark, err := strconv.Atoi(strings.TrimPrefix(linkName, consts.WiregaurdLinkNamePrefix))
	if err != nil {
		return -1, fmt.Errorf("failed to parse mark from link name (%s): %w", linkName, err)
	}
	return mark, err
}

// Similar syntax to utiliptables.Interface.EnsureRule, except you don't pass a table
// (you must write these rules under the line with the table name)
func writeRule(lines *bytes.Buffer, position string, chain utiliptables.Chain, args ...string) {
	fullArgs := append([]string{position, string(chain)}, args...)
	writeLine(lines, fullArgs...)
}

// Join all words with spaces, terminate with newline and write to buf.
func writeLine(lines *bytes.Buffer, words ...string) {
	lines.WriteString(strings.Join(words, " ") + "\n")
}
