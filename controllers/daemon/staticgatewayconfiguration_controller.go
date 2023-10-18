// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/Azure/kube-egress-gateway/pkg/iptableswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway node network according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	*azmanager.AzureManager
	TickerEvents chan event.GenericEvent
	Netlink      netlinkwrapper.Interface
	NetNS        netnswrapper.Interface
	IPTables     iptableswrapper.Interface
	WgCtrl       wgctrlwrapper.Interface
}

//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
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
	nodeMeta       *imds.InstanceMetadata
	lbMeta         *imds.LoadBalancerMetadata
	nodeTags       map[string]string
	vmssInstanceRE = regexp.MustCompile(`.*/subscriptions/(.+)/resourceGroups/(.+)/providers/Microsoft.Compute/virtualMachineScaleSets/(.+)/virtualMachines/(.+)`)
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
			return ctrl.Result{}, fmt.Errorf("failed to clean up orphaned network namespaces: %w", err)
		}
	}

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

	if !isReady(gwConfig) {
		// gateway setup hasn't completed yet
		return ctrl.Result{}, nil
	}

	if !applyToNode(gwConfig) {
		// gwConfig does not apply to this node
		return ctrl.Result{}, nil
	}

	if !gwConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// ignore: network namespace would be removed by regular cleanup
		return ctrl.Result{}, nil
	}

	// Reconcile gateway namespace
	return ctrl.Result{}, r.reconcile(ctx, gwConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Netlink = netlinkwrapper.NewNetLink()
	r.NetNS = netnswrapper.NewNetNS()
	r.IPTables = iptableswrapper.NewIPTables()
	r.WgCtrl = wgctrlwrapper.NewWgCtrl()
	controller, err := ctrl.NewControllerManagedBy(mgr).For(&egressgatewayv1alpha1.StaticGatewayConfiguration{}).Build(r)
	if err != nil {
		return err
	}
	return controller.Watch(&source.Channel{Source: r.TickerEvents}, &handler.EnqueueRequestForObject{})
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
	if err := r.reconcileIlbIPOnHost(ctx, gwConfig.Status.GatewayWireguardProfile.WireguardServerIp, false /* deleting */); err != nil {
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

	// add iptables rule in host namespace
	if err := r.addIPTablesRule(ctx, "-s", vmSecondaryIP+"/32",
		"-m", "comment", "--comment", consts.IPTablesRuleComment+getGatewayNamespaceName(gwConfig),
		"-j", "RETURN"); err != nil {
		return err
	}

	// configure gateway namespace (if not exists)
	if err := r.configureGatewayNamespace(ctx, gwConfig, privateKey, vmPrimaryIP, vmSecondaryIP); err != nil {
		return err
	}

	// update gateway status
	gwStatus := egressgatewayv1alpha1.GatewayNamespace{
		StaticGatewayConfiguration: fmt.Sprintf("%s/%s", gwConfig.Namespace, gwConfig.Name),
		NetnsName:                  getGatewayNamespaceName(gwConfig),
	}
	if err := r.updateGatewayNodeStatus(ctx, gwStatus, true /* add */); err != nil {
		return err
	}

	if err := healthprobe.AddGateway(string(gwConfig.GetUID())); err != nil {
		return err
	}

	log.Info("Gateway configuration reconciled")
	return nil
}

func (r *StaticGatewayConfigurationReconciler) cleanUp(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.Info("Cleaning up orphaned network namespaces")

	gwConfigList := &egressgatewayv1alpha1.StaticGatewayConfigurationList{}
	if err := r.List(ctx, gwConfigList); err != nil {
		return fmt.Errorf("failed to list staticGatewayConfigurations: %w", err)
	}
	existingNS := make(map[string]bool)
	for _, gwConfig := range gwConfigList.Items {
		existingNS[getGatewayNamespaceName(&gwConfig)] = true
	}

	netnsList, err := r.NetNS.ListNS()
	if err != nil {
		return fmt.Errorf("failed to list network namespaces: %w", err)
	}
	for _, netns := range netnsList {
		if _, ok := existingNS[netns]; !ok && strings.HasPrefix(netns, "gw-") {
			if err := r.ensureDeleted(ctx, netns); err != nil {
				log.Error(err, fmt.Sprintf("failed to remove namespace %s", netns))
			}
		}
	}

	log.Info("Network namespace cleanup completed")
	return nil
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleted(ctx context.Context, netns string) error {
	log := log.FromContext(ctx)

	// remove lb ip (if exists and if it's the last network namespace) from eth0
	netnsList, err := r.NetNS.ListNS()
	if err != nil {
		return fmt.Errorf("failed to list network namespace: %w", err)
	}
	exists := false
	for _, ns := range netnsList {
		if strings.HasPrefix(ns, "gw-") && ns != netns {
			exists = true
			break
		}
	}
	if !exists {
		ilbIP := getILBIPFromNamespaceName(netns)
		if err := r.reconcileIlbIPOnHost(ctx, ilbIP, true /* deleting */); err != nil {
			return err
		}
	}

	// delete iptables rule in host namespace
	if err := r.removeIPTablesRule(ctx, netns); err != nil {
		return err
	}

	// delete gateway namespace
	gwns, err := r.NetNS.GetNS(netns)
	if err == nil {
		gwns.Close()
		log.Info("Deleting network namespace", "namespace", netns)
		if err := r.NetNS.UnmountNS(netns); err != nil {
			return fmt.Errorf("failed to delete network namespace %s: %w", netns, err)
		}
	} else if _, ok := err.(ns.NSPathNotExistErr); !ok {
		return fmt.Errorf("failed to get network namespace %s: %w", netns, err)
	}

	// update gateway status
	gwStatus := egressgatewayv1alpha1.GatewayNamespace{
		NetnsName: netns,
	}
	if err := r.updateGatewayNodeStatus(ctx, gwStatus, false /* add */); err != nil {
		return err
	}

	gwUID := getGatewayUIDFromNamespaceName(netns)
	if err := healthprobe.RemoveGateway(gwUID); err != nil {
		return err
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) getWireguardPrivateKey(
	ctx context.Context,
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
) (*wgtypes.Key, error) {
	secretKey := &types.NamespacedName{
		Namespace: gwConfig.Namespace,
		Name:      gwConfig.Status.WireguardPrivateKeySecretRef.Name,
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, *secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to retrieve wireguard private key secret: %w", err)
	}

	wgPrivateKeyByte, ok := secret.Data[consts.WireguardSecretKeyName]
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
	matches := vmssInstanceRE.FindStringSubmatch(nodeMeta.Compute.ResourceID)
	if len(matches) != 5 {
		return "", "", fmt.Errorf("failed to parse vmss instance resource ID: %s", nodeMeta.Compute.ResourceID)
	}
	subscriptionID, resourceGroupName, vmssName, instanceID := matches[1], matches[2], matches[3], matches[4]
	if subscriptionID != r.SubscriptionID() {
		return "", "", fmt.Errorf("node subscription(%s) is different from configured subscription(%s)", subscriptionID, r.SubscriptionID())
	}
	vm, err := r.GetVMSSInstance(ctx, resourceGroupName, vmssName, instanceID)
	if err != nil {
		return "", "", fmt.Errorf("failed to get vmss instance: %w", err)
	}
	if vm.Properties == nil || vm.Properties.NetworkProfileConfiguration == nil {
		return "", "", fmt.Errorf("vm has empty network properties")
	}

	ipConfigName := gwConfig.Namespace + "_" + gwConfig.Name
	interfaces := vm.Properties.NetworkProfileConfiguration.NetworkInterfaceConfigurations
	nicName := ""
	for _, nic := range interfaces {
		if nic.Properties != nil && to.Val(nic.Properties.Primary) {
			nicName = to.Val(nic.Name)
			break
		}
	}

	if nicName == "" {
		return "", "", fmt.Errorf("failed to find primary interface of vmss instance(%s_%s)", vmssName, instanceID)
	}
	nic, err := r.GetVMSSInterface(ctx, resourceGroupName, vmssName, instanceID, nicName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get vmss instance primary interface: %w", err)
	}
	if nic.Properties == nil {
		return "", "", fmt.Errorf("nic has empty properties")
	}

	var primaryIP, ipConfigIP string
	for _, ipConfig := range nic.Properties.IPConfigurations {
		if ipConfig != nil && ipConfig.Properties != nil && strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) {
			if ipConfig.Properties.PrivateIPAddress == nil {
				return "", "", fmt.Errorf("ipConfig(%s) does not have private ip address", ipConfigName)
			}
			ipConfigIP = to.Val(ipConfig.Properties.PrivateIPAddress)
			log.Info("Found vm ip corresponding to gwConfig", "IP", ipConfigIP)
		} else if ipConfig != nil && ipConfig.Properties != nil && to.Val(ipConfig.Properties.Primary) {
			if ipConfig.Properties.PrivateIPAddress == nil {
				return "", "", fmt.Errorf("primary ipConfig does not have ip address")
			}
			primaryIP = to.Val(ipConfig.Properties.PrivateIPAddress)
			log.Info("Found vm primary ip", "IP", primaryIP)
		}
	}

	if primaryIP == "" || ipConfigIP == "" {
		return "", "", fmt.Errorf("failed to find vm ips, primaryIP(%s), ipConfigIP(%s)", primaryIP, ipConfigIP)
	}
	return primaryIP, ipConfigIP, nil
}

func isReady(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	wgProfile := gwConfig.Status.GatewayWireguardProfile
	return gwConfig.Status.EgressIpPrefix != "" && wgProfile.WireguardServerIp != "" &&
		wgProfile.WireguardServerPort != 0 && wgProfile.WireguardPublicKey != "" &&
		wgProfile.WireguardPrivateKeySecretRef != nil
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

func (r *StaticGatewayConfigurationReconciler) reconcileIlbIPOnHost(ctx context.Context, ilbIP string, deleting bool) error {
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

	ilbIpCidr := fmt.Sprintf("%s/%d", ilbIP, prefix)
	ilbIpNet, err := netlink.ParseIPNet(ilbIpCidr)
	if err != nil {
		return fmt.Errorf("failed to parse ILB IP address: %s", ilbIpCidr)
	}

	addresses, err := r.Netlink.AddrList(eth0, nl.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to retrieve IP addresses for eth0: %w", err)
	}

	addressPresent := false
	for _, address := range addresses {
		if address.IPNet.IP.Equal(ilbIpNet.IP) {
			addressPresent = true
			break
		}
	}

	if !addressPresent && !deleting {
		log.Info("Adding ILB IP to eth0", "ilb IP", ilbIpCidr)
		if err := r.Netlink.AddrAdd(eth0, &netlink.Addr{
			IPNet: ilbIpNet,
		}); err != nil {
			return fmt.Errorf("failed to add ILB IP to eth0: %w", err)
		}
	} else if addressPresent && deleting {
		log.Info("Removing ILB IP from eth0", "ilb IP", ilbIpCidr)
		if err := r.Netlink.AddrDel(eth0, &netlink.Addr{
			IPNet: ilbIpNet,
		}); err != nil {
			return fmt.Errorf("failed to delete ILB IP from eth0: %w", err)
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
	log := log.FromContext(ctx)

	gwNamespaceName := getGatewayNamespaceName(gwConfig)
	gwns, err := r.NetNS.GetNS(gwNamespaceName)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			log.Info("Creating new network namespace", "nsName", gwNamespaceName)
			gwns, err = r.NetNS.NewNS(gwNamespaceName)
			if err != nil {
				return fmt.Errorf("failed to create network namespace %s: %w", gwNamespaceName, err)
			}
		} else {
			return fmt.Errorf("failed to get network namespace %s: %w", gwNamespaceName, err)
		}
	}
	defer gwns.Close()

	if err := r.reconcileWireguardLink(ctx, gwns, gwConfig, privateKey); err != nil {
		return err
	}

	if err := r.reconcileVethPair(ctx, gwns, gwConfig, vmPrimaryIP, vmSecondaryIP); err != nil {
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

		if err := r.addIPTablesRule(ctx, "-o", consts.HostLinkName, "-j", "MASQUERADE"); err != nil {
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
	var wgLink netlink.Link
	var err error
	if err = gwns.Do(func(nn ns.NetNS) error {
		wgLink, err = r.Netlink.LinkByName(consts.WireguardLinkName)
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
		if err := r.createWireguardLink(gwns); err != nil {
			return fmt.Errorf("failed to create wireguard link: %w", err)
		}
	}

	return gwns.Do(func(nn ns.NetNS) error {
		wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
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
			ListenPort: to.Ptr(int(gwConfig.Status.WireguardServerPort)),
			PrivateKey: privateKey,
		}

		device, err := wgClient.Device(consts.WireguardLinkName)
		if err != nil {
			return fmt.Errorf("failed to get wireguard link configuration: %w", err)
		}

		if device.PrivateKey.String() != wgConfig.PrivateKey.String() || device.ListenPort != to.Val(wgConfig.ListenPort) {
			log.Info("Updating wireguard link config", "orig port", device.ListenPort, "cur port", to.Val(wgConfig.ListenPort),
				"private key difference", device.PrivateKey.String() != wgConfig.PrivateKey.String())
			err = wgClient.ConfigureDevice(consts.WireguardLinkName, wgConfig)
			if err != nil {
				return fmt.Errorf("failed to add peer to wireguard link: %w", err)
			}
		}
		return nil
	})
}

func (r *StaticGatewayConfigurationReconciler) createWireguardLink(gwns ns.NetNS) error {
	succeed := false
	attr := netlink.NewLinkAttrs()
	attr.Name = consts.WireguardLinkName
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

	wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
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
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	vmPrimaryIP string,
	vmSecondaryIP string,
) error {
	log := log.FromContext(ctx)
	if err := r.reconcileVethPairInHost(ctx, gwns, gwConfig, vmSecondaryIP); err != nil {
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
			err = r.Netlink.AddrReplace(hostLink, &hostLinkAddr)
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
	gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration,
	snatIP string,
) error {
	log := log.FromContext(ctx)
	succeed := false

	la := netlink.NewLinkAttrs()
	la.Name = getVethHostLinkName(gwConfig)

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

func (r *StaticGatewayConfigurationReconciler) addIPTablesRule(ctx context.Context, rulespec ...string) error {
	log := log.FromContext(ctx)
	ipt, err := r.IPTables.New()
	if err != nil {
		return fmt.Errorf("failed to get iptable: %w", err)
	}

	log.Info(fmt.Sprintf("Checking rule(%v) existence in nat table POSTROUTING chain", rulespec))
	exists, err := ipt.Exists(consts.NatTable, consts.PostRoutingChain, rulespec...)
	if err != nil {
		return fmt.Errorf("failed to check existence of iptables rule: %w", err)
	}

	if !exists {
		log.Info("Inserting rule at the beginning of nat table POSTROUTING chain")
		if err := ipt.Insert(consts.NatTable, consts.PostRoutingChain, 1, rulespec...); err != nil {
			return fmt.Errorf("failed to create iptables rule: %w", err)
		}
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) removeIPTablesRule(ctx context.Context, netns string) error {
	log := log.FromContext(ctx)
	ipt, err := r.IPTables.New()
	if err != nil {
		return fmt.Errorf("failed to get iptable: %w", err)
	}

	rules, err := ipt.List(consts.NatTable, consts.PostRoutingChain)
	if err != nil {
		return fmt.Errorf("failed to list rules in nat table POSTROUTING chain: %w", err)
	}

	for _, rule := range rules {
		if strings.Contains(rule, netns) {
			ruleSpec := strings.Split(rule, " ")
			for i, spec := range ruleSpec {
				if spec == "-s" {
					srcIP := ruleSpec[i+1]
					log.Info(fmt.Sprintf("Deleting rule(%s) from nat table POSTROUTING chain", rule))
					if err := ipt.Delete(consts.NatTable, consts.PostRoutingChain, "-s", srcIP,
						"-m", "comment", "--comment", consts.IPTablesRuleComment+netns,
						"-j", "RETURN"); err != nil {
						return fmt.Errorf("failed to delete iptables rule: %w", err)
					}
					break
				}
			}
		}
	}
	return nil
}

func (r *StaticGatewayConfigurationReconciler) updateGatewayNodeStatus(
	ctx context.Context,
	gwNamespace egressgatewayv1alpha1.GatewayNamespace,
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
					ReadyGatewayNamespaces: []egressgatewayv1alpha1.GatewayNamespace{gwNamespace},
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
		found := false
		for i, gwns := range gwStatus.Spec.ReadyGatewayNamespaces {
			if gwns.NetnsName == gwNamespace.NetnsName {
				if !add {
					changed = true
					gwStatus.Spec.ReadyGatewayNamespaces = append(gwStatus.Spec.ReadyGatewayNamespaces[:i], gwStatus.Spec.ReadyGatewayNamespaces[i+1:]...)
				}
				found = true
				break
			}
		}
		if add && !found {
			gwStatus.Spec.ReadyGatewayNamespaces = append(gwStatus.Spec.ReadyGatewayNamespaces, gwNamespace)
			changed = true
		}
		if !add {
			for i := len(gwStatus.Spec.ReadyPeerConfigurations) - 1; i >= 0; i = i - 1 {
				if gwStatus.Spec.ReadyPeerConfigurations[i].NetnsName == gwNamespace.NetnsName {
					changed = true
					gwStatus.Spec.ReadyPeerConfigurations = append(gwStatus.Spec.ReadyPeerConfigurations[:i], gwStatus.Spec.ReadyPeerConfigurations[i+1:]...)
				}
			}
		}
		if changed {
			log.Info("Updating gateway status object")
			if err := r.Update(ctx, gwStatus); err != nil {
				return fmt.Errorf("failed to update gwStatus object: %w", err)
			}
		}
	}
	return nil
}

func getGatewayNamespaceName(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) string {
	return fmt.Sprintf("gw-%s-%s", string(gwConfig.GetUID()), strings.Replace(gwConfig.Status.GatewayWireguardProfile.WireguardServerIp, ".", "_", -1))
}

func getVethHostLinkName(gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration) string {
	nsName := getGatewayNamespaceName(gwConfig)
	return nsName[:11]
}

func getGatewayUIDFromNamespaceName(netns string) string {
	return netns[strings.Index(netns, "-")+1 : strings.LastIndex(netns, "-")]
}

func getILBIPFromNamespaceName(netns string) string {
	return strings.Replace(netns[strings.LastIndex(netns, "-")+1:], "_", ".", -1)
}
