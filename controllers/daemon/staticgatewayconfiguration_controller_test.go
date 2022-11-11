package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/interfaceclient/mockinterfaceclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient/mockvmssvmclient"
	"github.com/Azure/kube-egress-gateway/pkg/iptableswrapper/mockiptableswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper/mocknetlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper/mockwgctrlwrapper"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"github.com/vishvananda/netns"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/consts"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	testName         = "test"
	testNamespace    = "testns"
	testNodepoolName = "testgw"
	testUID          = "1234567890"
	vmssRG           = "vmssRG"
	vmssName         = "vmssName"
	privK            = "GHuMwljFfqd2a7cs6BaUOmHflK23zME8VNvC5B37S3k="
	pubK             = "aPxGwq8zERHQ3Q1cOZFdJ+cvJX5Ka4mLN38AyYKYF10="
	ilbIPCidr        = "10.0.0.4/31"
)

var _ = Describe("Daemon StaticGatewayConfiguration controller unit tests", func() {
	var (
		s       = scheme.Scheme
		r       *StaticGatewayConfigurationReconciler
		az      *azmanager.AzureManager
		mclient *mockwgctrlwrapper.MockClient
		mtable  *mockiptableswrapper.MockIpTables
	)

	Context("Reconcile", func() {
		var (
			req          reconcile.Request
			res          reconcile.Result
			cl           client.Client
			reconcileErr error
			gwConfig     *kubeegressgatewayv1alpha1.StaticGatewayConfiguration
		)

		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       testUID,
				},
				Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayNodepoolName: testNodepoolName,
				},
			}
			s.AddKnownTypes(kubeegressgatewayv1alpha1.GroupVersion, gwConfig)
		})

		When("gwConfig is not found", func() {
			It("should not report error", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig is not ready", func() {
			It("should not do anything", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(gwConfig).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig does not apply to the node", func() {
			It("should not do anything", func() {
				gwConfig.Status = getTestGwConfigStatus()
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(gwConfig).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})

			It("should report error if secret is not found", func() {
				gwConfig.Status = getTestGwConfigStatus()
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(gwConfig).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				nodeTags = map[string]string{consts.AKSNodepoolTagKey: testNodepoolName}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(apierrors.IsNotFound(reconcileErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		Context("Test reconcile", func() {
			BeforeEach(func() {
				req = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      testName,
						Namespace: testNamespace,
					},
				}
				gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
						UID:       testUID,
					},
					Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
						GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  vmssRG,
							VMSSName:           vmssName,
							PublicIpPrefixSize: 31,
						},
					},
					Status: getTestGwConfigStatus(),
				}
				s.AddKnownTypes(kubeegressgatewayv1alpha1.GroupVersion, gwConfig)
				nodeMeta = &imds.InstanceMetadata{
					Compute: &imds.ComputeMetadata{
						VMScaleSetName:    vmssName,
						ResourceGroupName: vmssRG,
						ResourceID: "/subscriptions/testSub/resourceGroups/" + vmssRG + "/providers/" +
							"Microsoft.Compute/virtualMachineScaleSets/" + vmssName + "/virtualMachines/0",
						Tags: "a:b; c : d ;e",
					},
					Network: &imds.NetworkMetadata{
						Interface: []imds.NetworkInterface{
							{IPv4: imds.IPData{Subnet: []imds.Subnet{{Prefix: "31"}}}},
						},
					},
				}
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Data: map[string][]byte{
						consts.WireguardSecretKeyName: []byte(privK),
					},
				}
				mctrl := gomock.NewController(GinkgoT())
				az = getMockAzureManager(mctrl)
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(gwConfig, secret).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				r.Netlink = mocknetlinkwrapper.NewMockInterface(mctrl)
				r.NetNS = mocknetnswrapper.NewMockInterface(mctrl)
				r.IPTables = mockiptableswrapper.NewMockInterface(mctrl)
				r.WgCtrl = mockwgctrlwrapper.NewMockInterface(mctrl)
				mclient = mockwgctrlwrapper.NewMockClient(mctrl)
				mtable = mockiptableswrapper.NewMockIpTables(mctrl)
			})

			It("should parse node tags correctly", func() {
				expected := map[string]string{
					"a": "b",
					"c": "d",
				}
				got := parseNodeTags()
				Expect(got).To(Equal(expected))
			})

			It("should add ilb ip to eth0", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil)
				mnl.EXPECT().AddrAdd(eth0, &netlink.Addr{IPNet: getIPNet(ilbIPCidr)}).Return(nil)
				err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig, false)
				Expect(err).To(BeNil())
			})

			It("should not add ilb ip to eth0 if it already exists", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil)
				err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig, false)
				Expect(err).To(BeNil())
			})

			It("should delete ilb ip from eth0", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil)
				mnl.EXPECT().AddrDel(eth0, &netlink.Addr{IPNet: getIPNet(ilbIPCidr)}).Return(nil)
				err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig, true)
				Expect(err).To(BeNil())
			})

			It("should not delete ilb ip to eth0 if it does not exist", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil)
				err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig, true)
				Expect(err).To(BeNil())
			})

			It("should report when errors happen during adding ilb ip", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should retrieve vm ips", func() {
				vm, nic := getTestVM(), getTestNic()
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().Get(gomock.Any(), vmssRG, vmssName, "0", gomock.Any()).Return(vm, nil)
				mockInterfaceClient := az.InterfaceClient.(*mockinterfaceclient.MockInterface)
				mockInterfaceClient.EXPECT().
					GetVirtualMachineScaleSetNetworkInterface(gomock.Any(), vmssRG, vmssName, "0", "primary", gomock.Any()).
					Return(nic, nil)
				primaryIP, secondaryIP, err := r.getVMIP(context.TODO(), gwConfig)
				Expect(primaryIP).To(Equal("10.0.0.5"))
				Expect(secondaryIP).To(Equal("10.0.0.6"))
				Expect(err).To(BeNil())
			})

			It("should report when errors happen during retrieving vm ips", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().Get(gomock.Any(), vmssRG, vmssName, "0", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should remove secondary ip from eth0", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil)
				mnl.EXPECT().AddrDel(eth0, &netlink.Addr{IPNet: getIPNet(ilbIPCidr)}).Return(nil)
				err := r.removeSecondaryIpFromHost(context.TODO(), "10.0.0.4")
				Expect(err).To(BeNil())
			})

			It("should not do anything if secondary ip is not applied to eth0", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil)
				err := r.removeSecondaryIpFromHost(context.TODO(), "10.0.0.4")
				Expect(err).To(BeNil())
			})

			It("should report when errors happen during removing secondary ip", func() {
				vm, nic := getTestVM(), getTestNic()
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockInterfaceClient := az.InterfaceClient.(*mockinterfaceclient.MockInterface)
				gomock.InOrder(
					mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
					mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil),
					mockVMSSVMClient.EXPECT().Get(gomock.Any(), vmssRG, vmssName, "0", gomock.Any()).Return(vm, nil),
					mockInterfaceClient.EXPECT().
						GetVirtualMachineScaleSetNetworkInterface(gomock.Any(), vmssRG, vmssName, "0", "primary", gomock.Any()).
						Return(nic, nil),
					mnl.EXPECT().LinkByName("eth0").Return(eth0, fmt.Errorf("failed")),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should create new network namespace, wireguard interface and veth pair, routes, and iptables rules", func() {
				pk, _ := wgtypes.ParseKey(privK)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				mipt := r.IPTables.(*mockiptableswrapper.MockInterface)
				la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
				la1.Name = "wg0"
				la2.Name = "gw-12345678"
				wg0 := &netlink.Wireguard{LinkAttrs: la1}
				veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
				host0 := &netlink.Veth{}
				loop := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}}
				device := &wgtypes.Device{Name: "wg0"}
				origns, gwns, gwns2 := netns.NsHandle(-1), netns.NsHandle(-2), netns.NsHandle(-3) // use negative value for Close()
				gomock.InOrder(
					// create network namespace
					mns.EXPECT().Get().Return(origns, nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(netns.NsHandle(0), os.ErrNotExist),
					mns.EXPECT().NewNamed("gw-1234567890").Return(gwns, nil),
					mns.EXPECT().Set(gwns).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, netlink.LinkNotFoundError{}),
					// create wireguard link, wg0
					mns.EXPECT().Get().Return(gwns2, nil),
					mns.EXPECT().Set(origns).Return(nil),
					mnl.EXPECT().LinkAdd(wg0).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					mnl.EXPECT().LinkSetNsFd(wg0, int(gwns)).Return(nil),
					mns.EXPECT().Set(gwns2).Return(nil),
					// add address to wg0
					mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
					mnl.EXPECT().AddrAdd(wg0, &netlink.Addr{IPNet: getIPNet(consts.GatewayIP)}),
					mnl.EXPECT().LinkSetUp(wg0).Return(nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mclient.EXPECT().Device("wg0").Return(device, nil),
					mclient.EXPECT().ConfigureDevice("wg0", wgtypes.Config{ListenPort: to.Ptr[int](6000), PrivateKey: &pk}).Return(nil),
					// add veth pair in host
					mns.EXPECT().Get().Return(gwns2, nil),
					mns.EXPECT().Set(origns).Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, netlink.LinkNotFoundError{}),
					mnl.EXPECT().LinkAdd(veth).Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, nil),
					mnl.EXPECT().LinkSetUp(veth).Return(nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.6/32")}).Return(nil),
					mnl.EXPECT().LinkByName("host0").Return(host0, nil),
					mnl.EXPECT().LinkSetNsFd(host0, int(gwns)).Return(nil),
					mns.EXPECT().Set(gwns2).Return(nil),
					// add address and routes in gw namespace
					mnl.EXPECT().LinkByName("host0").Return(host0, nil),
					mnl.EXPECT().AddrList(host0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
					mnl.EXPECT().AddrReplace(host0, &netlink.Addr{IPNet: getIPNet("10.0.0.6/32")}).Return(nil),
					mnl.EXPECT().LinkSetUp(host0).Return(nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.5/32")}).Return(nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Gw: net.ParseIP("10.0.0.5")}).Return(nil),
					mnl.EXPECT().LinkByName("lo").Return(loop, nil),
					mnl.EXPECT().LinkSetUp(loop).Return(nil),
					// setup iptables rule
					mipt.EXPECT().New().Return(mtable, nil),
					mtable.EXPECT().AppendUnique("nat", "POSTROUTING", "-o", "host0", "-j", "MASQUERADE").Return(nil),
					// back to host namespace
					mns.EXPECT().Set(origns).Return(nil),
				)
				err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
				Expect(err).To(BeNil())
			})

			It("should not change anything when setup is complete", func() {
				pk, _ := wgtypes.ParseKey(privK)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				mipt := r.IPTables.(*mockiptableswrapper.MockInterface)
				la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
				la1.Name = "wg0"
				la2.Name = "gw-12345678"
				wg0 := &netlink.Wireguard{LinkAttrs: la1}
				veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
				host0 := &netlink.Veth{}
				loop := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}}
				device := &wgtypes.Device{Name: "wg0", ListenPort: 6000, PrivateKey: pk}
				origns, gwns, gwns2 := netns.NsHandle(-1), netns.NsHandle(-2), netns.NsHandle(-3) // use negative value for Close()
				gomock.InOrder(
					// create network namespace
					mns.EXPECT().Get().Return(origns, nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(gwns, nil),
					mns.EXPECT().Set(gwns).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					// check address and wg config for wg0
					mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(consts.GatewayIP)}}, nil),
					mnl.EXPECT().LinkSetUp(wg0).Return(nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mclient.EXPECT().Device("wg0").Return(device, nil),
					// check veth pair in host
					mns.EXPECT().Get().Return(gwns2, nil),
					mns.EXPECT().Set(origns).Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, nil),
					mnl.EXPECT().LinkSetUp(veth).Return(nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.6/32")}}, nil),
					mnl.EXPECT().LinkByName("host0").Return(host0, netlink.LinkNotFoundError{}),
					mns.EXPECT().Set(gwns2).Return(nil),
					// check address and routes in gw namespace
					mnl.EXPECT().LinkByName("host0").Return(host0, nil),
					mnl.EXPECT().AddrList(host0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet("10.0.0.6/32")}}, nil),
					mnl.EXPECT().LinkSetUp(host0).Return(nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{
						{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.5/32")},
						{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Gw: net.ParseIP("10.0.0.5")},
					}, nil),
					mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{
						{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.5/32")},
						{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Gw: net.ParseIP("10.0.0.5")},
					}, nil),
					mnl.EXPECT().LinkByName("lo").Return(loop, nil),
					mnl.EXPECT().LinkSetUp(loop).Return(nil),
					// check iptables rule
					mipt.EXPECT().New().Return(mtable, nil),
					mtable.EXPECT().AppendUnique("nat", "POSTROUTING", "-o", "host0", "-j", "MASQUERADE").Return(nil),
					// back to host namespace
					mns.EXPECT().Set(origns).Return(nil),
				)
				err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
				Expect(err).To(BeNil())
			})

			It("should delete wireguard link if any setup fails", func() {
				pk, _ := wgtypes.ParseKey(privK)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
				la1.Name = "wg0"
				la2.Name = "gw-12345678"
				wg0 := &netlink.Wireguard{LinkAttrs: la1}
				origns, gwns, gwns2 := netns.NsHandle(-1), netns.NsHandle(-2), netns.NsHandle(-3) // use negative value for Close()
				gomock.InOrder(
					// create network namespace
					mns.EXPECT().Get().Return(origns, nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(gwns, nil),
					mns.EXPECT().Set(gwns).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, netlink.LinkNotFoundError{}),
					// create wireguard link, wg0
					mns.EXPECT().Get().Return(gwns2, nil),
					mns.EXPECT().Set(origns).Return(nil),
					mnl.EXPECT().LinkAdd(wg0).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					mnl.EXPECT().LinkSetNsFd(wg0, int(gwns)).Return(fmt.Errorf("failed")),
					mnl.EXPECT().LinkDel(wg0).Return(nil),
					mns.EXPECT().Set(gwns2).Return(nil),
					// back to host namespace
					mns.EXPECT().Set(origns).Return(nil),
				)
				err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
				Expect(errors.Unwrap(errors.Unwrap(err))).To(Equal(fmt.Errorf("failed")))
			})

			It("should delete veth pair if any setup fails", func() {
				pk, _ := wgtypes.ParseKey(privK)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
				la1.Name = "wg0"
				la2.Name = "gw-12345678"
				wg0 := &netlink.Wireguard{LinkAttrs: la1}
				veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
				device := &wgtypes.Device{Name: "wg0", ListenPort: 6000, PrivateKey: pk}
				origns, gwns, gwns2 := netns.NsHandle(-1), netns.NsHandle(-2), netns.NsHandle(-3) // use negative value for Close()
				gomock.InOrder(
					// create network namespace
					mns.EXPECT().Get().Return(origns, nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(gwns, nil),
					mns.EXPECT().Set(gwns).Return(nil),
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					// check address and wg config for wg0
					mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(consts.GatewayIP)}}, nil),
					mnl.EXPECT().LinkSetUp(wg0).Return(nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mclient.EXPECT().Device("wg0").Return(device, nil),
					// add veth pair in host
					mns.EXPECT().Get().Return(gwns2, nil),
					mns.EXPECT().Set(origns).Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, netlink.LinkNotFoundError{}),
					mnl.EXPECT().LinkAdd(veth).Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, nil),
					mnl.EXPECT().LinkSetUp(veth).Return(fmt.Errorf("failed")),
					mnl.EXPECT().LinkDel(veth).Return(nil),
					mns.EXPECT().Set(gwns2).Return(nil),
					// back to host namespace
					mns.EXPECT().Set(origns).Return(nil),
				)
				err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
				Expect(errors.Unwrap(errors.Unwrap(err))).To(Equal(fmt.Errorf("failed")))
			})
		})

		Context("Test reconcile deletion", func() {
			BeforeEach(func() {
				req = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      testName,
						Namespace: testNamespace,
					},
				}
				gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:              testName,
						Namespace:         testNamespace,
						UID:               testUID,
						DeletionTimestamp: to.Ptr(metav1.Now()),
					},
					Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
						GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  vmssRG,
							VMSSName:           vmssName,
							PublicIpPrefixSize: 31,
						},
					},
					Status: getTestGwConfigStatus(),
				}
				s.AddKnownTypes(kubeegressgatewayv1alpha1.GroupVersion, gwConfig)
				nodeMeta = &imds.InstanceMetadata{
					Compute: &imds.ComputeMetadata{
						VMScaleSetName:    vmssName,
						ResourceGroupName: vmssRG,
					},
					Network: &imds.NetworkMetadata{
						Interface: []imds.NetworkInterface{
							{IPv4: imds.IPData{Subnet: []imds.Subnet{{Prefix: "31"}}}},
						},
					},
				}
				mctrl := gomock.NewController(GinkgoT())
				az = getMockAzureManager(mctrl)
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(gwConfig).Build()
				r = &StaticGatewayConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				r.Netlink = mocknetlinkwrapper.NewMockInterface(mctrl)
				r.NetNS = mocknetnswrapper.NewMockInterface(mctrl)
			})

			It("should delete ilb ip from eth0, delete gateway namespace and veth link", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				veth := &netlink.Veth{PeerName: "host0"}
				gwns := netns.NsHandle(-1) // use negative value for Close()
				gomock.InOrder(
					mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
					mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet(ilbIPCidr)}}, nil),
					mnl.EXPECT().AddrDel(eth0, &netlink.Addr{IPNet: getIPNet(ilbIPCidr)}).Return(nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(gwns, nil),
					mns.EXPECT().DeleteNamed("gw-1234567890").Return(nil),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, nil),
					mnl.EXPECT().LinkDel(veth).Return(nil),
				)
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})

			It("should not do anything if it's already been cleaned", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				veth := &netlink.Veth{PeerName: "host0"}
				gwns := netns.NsHandle(-1) // use negative value for Close()
				gomock.InOrder(
					mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
					mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
					mns.EXPECT().GetFromName("gw-1234567890").Return(gwns, os.ErrNotExist),
					mnl.EXPECT().LinkByName("gw-12345678").Return(veth, netlink.LinkNotFoundError{}),
				)
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})

			It("should report any error", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
				mnl.EXPECT().LinkByName("eth0").Return(eth0, fmt.Errorf("failed"))
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})
	})
})

func getMockAzureManager(ctrl *gomock.Controller) *azmanager.AzureManager {
	conf := &config.CloudConfig{
		Cloud:            "AzureTest",
		Location:         "location",
		SubscriptionID:   "testSub",
		UserAgent:        "testUserAgent",
		ResourceGroup:    "rg",
		LoadBalancerName: "lb",
	}
	az, _ := azmanager.CreateAzureManager(conf, azureclients.NewMockAzureClientsFactory(ctrl))
	return az
}

func getTestGwConfigStatus() kubeegressgatewayv1alpha1.StaticGatewayConfigurationStatus {
	return kubeegressgatewayv1alpha1.StaticGatewayConfigurationStatus{
		PublicIpPrefix: "1.2.3.4/31",
		GatewayWireguardProfile: kubeegressgatewayv1alpha1.GatewayWireguardProfile{
			WireguardServerIP:   "10.0.0.4",
			WireguardServerPort: 6000,
			WireguardPublicKey:  pubK,
			WireguardPrivateKeySecretRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       testName,
			},
		},
	}
}

func getIPNet(ipCidr string) *net.IPNet {
	ipNet, _ := netlink.ParseIPNet(ipCidr)
	return ipNet
}

func getTestVM() *compute.VirtualMachineScaleSetVM {
	return &compute.VirtualMachineScaleSetVM{
		Properties: &compute.VirtualMachineScaleSetVMProperties{
			NetworkProfileConfiguration: &compute.VirtualMachineScaleSetVMNetworkProfileConfiguration{
				NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{
					{
						Name: to.Ptr("primary"),
						Properties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
							Primary: to.Ptr(true),
						},
					},
				},
			},
		},
	}
}

func getTestNic() *network.Interface {
	return &network.Interface{
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: []*network.InterfaceIPConfiguration{
				{
					Properties: &network.InterfaceIPConfigurationPropertiesFormat{
						Primary:          to.Ptr(true),
						PrivateIPAddress: to.Ptr("10.0.0.5"),
					},
				},
				{
					Name: to.Ptr(testNamespace + "_" + testName),
					Properties: &network.InterfaceIPConfigurationPropertiesFormat{
						PrivateIPAddress: to.Ptr("10.0.0.6"),
					},
				},
			},
		},
	}
}
