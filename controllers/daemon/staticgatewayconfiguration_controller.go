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
	"regexp"
	"runtime"
	"strconv"
	"strings"

	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/consts"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/Azure/kube-egress-gateway/pkg/iptableswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"github.com/vishvananda/netns"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway node network according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	Scheme *k8sruntime.Scheme
	*azmanager.AzureManager
	Netlink  netlinkwrapper.Interface
	NetNS    netnswrapper.Interface
	IPTables iptableswrapper.Interface
	WgCtrl   wgctrlwrapper.Interface
}

//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations,verbs=get;list;watch
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

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
	nodeMeta           *imds.InstanceMetadata
	lbMeta             *imds.LoadBalancerMetadata
	nodeTags           map[string]string
	vmssInstanceRE     = regexp.MustCompile(`.*/subscriptions/(.+)/resourceGroups/(.+)/providers/Microsoft.Compute/virtualMachineScaleSets/(.+)/virtualMachines/(.+)`)
	gwNamespaceName    string
	gwVethHostLinkName string
)

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

	if !isReady(gwConfig) {
		// gateway setup hasn't completed yet
		return ctrl.Result{}, nil
	}

	if !applyToNode(gwConfig) {
		// gwConfig does not apply to this node
		return ctrl.Result{}, nil
	}

	gwNamespaceName = fmt.Sprintf("gw-%s", string(gwConfig.GetUID()))
	gwVethHostLinkName = gwNamespaceName[:11]

	if !gwConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Clean up gateway namespace
		return r.ensureDeleted(ctx, gwConfig)
	}

	// Reconcile gateway namespace
	return r.reconcile(ctx, gwConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
	r.Netlink = netlinkwrapper.NewNetLink()
	r.NetNS = netnswrapper.NewNetNS()
	r.IPTables = iptableswrapper.NewIPTables()
	r.WgCtrl = wgctrlwrapper.NewWgCtrl()
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeegressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Complete(r)
}

func (r *StaticGatewayConfigurationReconciler) reconcile(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling gateway configuration")

	// get wireguard private key from secret
	privateKey, err := r.getWireguardPrivateKey(ctx, gwConfig)
	if err != nil {
		return ctrl.Result{}, err
	}

	// add lb ip (if not exists) to eth0
	if err := r.reconcileIlbIPOnHost(ctx, gwConfig, false /* deleting */); err != nil {
		return ctrl.Result{}, err
	}

	// remove secondary ip from eth0
	vmPrimaryIP, vmSecondaryIP, err := r.getVMIP(ctx, gwConfig)
	if err != nil {
		return ctrl.Result{}, err
	}
	if err := r.removeSecondaryIpFromHost(ctx, vmSecondaryIP); err != nil {
		return ctrl.Result{}, err
	}

	// configure gateway namespace (if not exists)
	if err := r.configureGatewayNamespace(ctx, gwConfig, privateKey, vmPrimaryIP, vmSecondaryIP); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Gateway configuration reconciled")
	return ctrl.Result{}, nil
}

func (r *StaticGatewayConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling gateway configuration deletion")

	// remove lb ip (if exists) from eth0
	if err := r.reconcileIlbIPOnHost(ctx, gwConfig, true /* deleting */); err != nil {
		return ctrl.Result{}, err
	}

	// delete gateway namespace
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	gwns, err := r.NetNS.GetFromName(gwNamespaceName)
	if err == nil {
		gwns.Close()
		log.Info("Deleting network namespace", "namespace", gwNamespaceName)
		if err := r.NetNS.DeleteNamed(gwNamespaceName); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete network namespace %s: %w", gwNamespaceName, err)
		}
	} else if !os.IsNotExist(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get network namespace %s: %w", gwNamespaceName, err)
	}

	// delete link in host namespace in case it still exists
	orphanedLink, err := r.Netlink.LinkByName(gwVethHostLinkName)
	if err == nil {
		log.Info("Deleting orphaned veth interface in host namespace")
		if err := r.Netlink.LinkDel(orphanedLink); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete orphaned veth link: %w", err)
		}
	} else if _, ok := err.(netlink.LinkNotFoundError); !ok {
		return ctrl.Result{}, fmt.Errorf("failed to get orphaned veth interface in host namespace: %w", err)
	}

	log.Info("Gateway configuration deletion reconciled")
	return ctrl.Result{}, nil
}

func (r *StaticGatewayConfigurationReconciler) getWireguardPrivateKey(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
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
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
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
	vm, err := r.GetVMSSInstance(resourceGroupName, vmssName, instanceID)
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
	nic, err := r.GetVMSSInterface(resourceGroupName, vmssName, instanceID, nicName)
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

func isReady(gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	wgProfile := gwConfig.Status.GatewayWireguardProfile
	return gwConfig.Status.PublicIpPrefix != "" && wgProfile.WireguardServerIP != "" &&
		wgProfile.WireguardServerPort != 0 && wgProfile.WireguardPublicKey != "" &&
		wgProfile.WireguardPrivateKeySecretRef != nil
}

func applyToNode(gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration) bool {
	if gwConfig.Spec.GatewayNodepoolName != "" {
		name, ok := nodeTags[consts.AKSNodepoolTagKey]
		return ok && strings.EqualFold(name, gwConfig.Spec.GatewayNodepoolName)
	} else {
		vmssProfile := gwConfig.Spec.GatewayVMSSProfile
		return strings.EqualFold(vmssProfile.VMSSName, nodeMeta.Compute.VMScaleSetName) &&
			strings.EqualFold(vmssProfile.VMSSResourceGroup, nodeMeta.Compute.ResourceGroupName)
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

func (r *StaticGatewayConfigurationReconciler) reconcileIlbIPOnHost(
	ctx context.Context,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
	deleting bool,
) error {
	log := log.FromContext(ctx)
	eth0, err := r.Netlink.LinkByName("eth0")
	if err != nil {
		return fmt.Errorf("failed to retrieve link eth0: %w", err)
	}

	wgProfile := gwConfig.Status.GatewayWireguardProfile

	if len(nodeMeta.Network.Interface) == 0 || len(nodeMeta.Network.Interface[0].IPv4.Subnet) == 0 {
		return fmt.Errorf("imds does not provide subnet information about the node")
	}
	prefix, err := strconv.Atoi(nodeMeta.Network.Interface[0].IPv4.Subnet[0].Prefix)
	if err != nil {
		return fmt.Errorf("failed to retrieve and parse prefix: %w", err)
	}

	ilbIpCidr := fmt.Sprintf("%s/%d", wgProfile.WireguardServerIP, prefix)
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
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
	privateKey *wgtypes.Key,
	vmPrimaryIP string,
	vmSecondaryIP string,
) error {
	log := log.FromContext(ctx)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	origns, _ := r.NetNS.Get()
	defer func() { _ = r.NetNS.Set(origns); origns.Close() }()

	gwns, err := r.NetNS.GetFromName(gwNamespaceName)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info("Creating new network namespace", "nsName", gwNamespaceName)
			gwns, err = r.NetNS.NewNamed(gwNamespaceName)
			if err != nil {
				return fmt.Errorf("failed to create network namespace %s: %w", gwNamespaceName, err)
			}
		} else {
			return fmt.Errorf("failed to get network namespace %s: %w", gwNamespaceName, err)
		}
	}
	defer gwns.Close()
	_ = r.NetNS.Set(gwns)
	log.Info(fmt.Sprintf("Setting namespace to %s", gwNamespaceName))

	// we are in gw namespace now
	if err := r.reconcileWireguardLink(ctx, &origns, &gwns, gwConfig, privateKey); err != nil {
		return err
	}

	if err := r.reconcileVethPair(ctx, &origns, &gwns, gwConfig, vmPrimaryIP, vmSecondaryIP); err != nil {
		return err
	}

	looplink, err := r.Netlink.LinkByName("lo")
	if err != nil {
		return fmt.Errorf("failed to retrieve link lo: %w", err)
	}
	if err := r.Netlink.LinkSetUp(looplink); err != nil {
		return fmt.Errorf("failed to set lo up: %w", err)
	}

	if err := r.reconcileIPTablesRule(ctx); err != nil {
		return err
	}
	return nil
}

// this function is called in gwNamespace
func (r *StaticGatewayConfigurationReconciler) reconcileWireguardLink(
	ctx context.Context,
	origns, gwns *netns.NsHandle,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
	privateKey *wgtypes.Key,
) error {
	log := log.FromContext(ctx)
	wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
	if _, ok := err.(netlink.LinkNotFoundError); ok {
		log.Info("Creating wireguard link")
		wgLink, err = r.createWireguardLink(origns, gwns)
		if err != nil {
			return fmt.Errorf("failed to create wireguard link: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get wireguard link in gateway namespace: %w", err)
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
}

func (r *StaticGatewayConfigurationReconciler) createWireguardLink(origns, gwns *netns.NsHandle) (netlink.Link, error) {
	curns, _ := r.NetNS.Get()
	defer func() { _ = r.NetNS.Set(curns); curns.Close() }()
	// create wg link in host namespace first
	_ = r.NetNS.Set(*origns)
	succeed := false
	attr := netlink.NewLinkAttrs()
	attr.Name = consts.WireguardLinkName
	wg := &netlink.Wireguard{LinkAttrs: attr}
	err := r.Netlink.LinkAdd(wg)
	if err != nil {
		return nil, fmt.Errorf("failed to create wireguard link: %w", err)
	}
	defer func() {
		if !succeed {
			_ = r.Netlink.LinkDel(wg)
		}
	}()

	wgLink, err := r.Netlink.LinkByName(consts.WireguardLinkName)
	if err != nil {
		return nil, fmt.Errorf("failed to get wireguard link in host namespace: %w", err)
	}

	if err := r.Netlink.LinkSetNsFd(wgLink, int(*gwns)); err != nil {
		return nil, fmt.Errorf("failed to move wireguard link to gateway namespace: %w", err)
	}

	succeed = true
	return wgLink, nil
}

// this function is called in gateway namespace
func (r *StaticGatewayConfigurationReconciler) reconcileVethPair(
	ctx context.Context,
	origns, gwns *netns.NsHandle,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
	vmPrimaryIP string,
	vmSecondaryIP string,
) error {
	log := log.FromContext(ctx)
	if err := r.reconcileVethPairInHost(ctx, origns, gwns, gwConfig, vmSecondaryIP); err != nil {
		return fmt.Errorf("failed to reconcile veth pair in host namespace: %w", err)
	}

	hostLink, err := r.Netlink.LinkByName(consts.HostLinkName)
	if err != nil {
		return fmt.Errorf("failed to get host link in gateway namespace: %w", err)
	}

	snatIPNet, err := netlink.ParseIPNet(vmSecondaryIP + "/32")
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

	vmSnatCidr, err := netlink.ParseIPNet(vmPrimaryIP + "/32")
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
}

func (r *StaticGatewayConfigurationReconciler) reconcileVethPairInHost(
	ctx context.Context,
	origns, gwns *netns.NsHandle,
	gwConfig *kubeegressgatewayv1alpha1.StaticGatewayConfiguration,
	snatIP string,
) error {
	log := log.FromContext(ctx)
	curns, _ := r.NetNS.Get()
	defer func() { _ = r.NetNS.Set(curns); curns.Close() }()
	_ = r.NetNS.Set(*origns)

	succeed := false

	la := netlink.NewLinkAttrs()
	la.Name = gwVethHostLinkName

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

	snatIPNet, err := netlink.ParseIPNet(snatIP + "/32")
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
		if err := r.Netlink.LinkSetNsFd(hostLink, int(*gwns)); err != nil {
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

func (r *StaticGatewayConfigurationReconciler) reconcileIPTablesRule(ctx context.Context) error {
	log := log.FromContext(ctx)
	ipt, err := r.IPTables.New()
	if err != nil {
		return fmt.Errorf("failed to get iptable: %w", err)
	}

	log.Info("Checking and creating nat rule (if not exists) in POSTROUTING chain")
	if err := ipt.AppendUnique(consts.NatTable, consts.PostRoutingChain, "-o", consts.HostLinkName, "-j", "MASQUERADE"); err != nil {
		return fmt.Errorf("failed to check or create nat rule in POSTROUTING chain: %w", err)
	}
	return nil
}
