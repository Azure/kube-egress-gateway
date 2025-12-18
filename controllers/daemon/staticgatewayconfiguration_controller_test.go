// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"go.uber.org/mock/gomock"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	utiliptables "k8s.io/kubernetes/pkg/util/iptables"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	fakeiptables "github.com/Azure/kube-egress-gateway/pkg/iptableswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper/mocknetlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper/mockwgctrlwrapper"
)

const (
	testName            = "test"
	testNamespace       = "testns"
	testSecretNamespace = "testns2"
	testNodepoolName    = "testgw"
	testPodNamespace    = "testns2"
	testNodeName        = "testNode"
	testUID             = "1234567890"
	vmssRG              = "vmssRG"
	vmssName            = "vmssName"
	privK               = "GHuMwljFfqd2a7cs6BaUOmHflK23zME8VNvC5B37S3k="
	pubK                = "aPxGwq8zERHQ3Q1cOZFdJ+cvJX5Ka4mLN38AyYKYF10="
	ilbIP               = "10.0.0.4"
	ilbIPCidr           = "10.0.0.4/32"
	natBuiltinChains    = `*nat
:PREROUTING - [0:0]
:INPUT - [0:0]
:OUTPUT - [0:0]
:POSTROUTING - [0:0]`
)

var _ = Describe("Daemon StaticGatewayConfiguration controller unit tests", func() {
	var (
		r            *StaticGatewayConfigurationReconciler
		req          reconcile.Request
		res          reconcile.Result
		reconcileErr error
		gwConfig     *egressgatewayv1alpha1.StaticGatewayConfiguration
		mclient      *mockwgctrlwrapper.MockClient
		node         = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNodeName}}
	)

	getTestReconciler := func(objects ...runtime.Object) {
		mctrl := gomock.NewController(GinkgoT())
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objects...).Build()
		r = &StaticGatewayConfigurationReconciler{Client: cl, LBProbeServer: healthprobe.NewLBProbeServer(1000)}
		r.Netlink = mocknetlinkwrapper.NewMockInterface(mctrl)
		r.NetNS = mocknetnswrapper.NewMockInterface(mctrl)
		r.IPTables = fakeiptables.NewFake()
		r.WgCtrl = mockwgctrlwrapper.NewMockInterface(mctrl)
		mclient = mockwgctrlwrapper.NewMockClient(mctrl)
	}

	Context("Test ignore reconcile", func() {
		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			gwConfig = &egressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       testUID,
				},
				Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayNodepoolName: testNodepoolName,
				},
			}
		})

		When("gwConfig is not found", func() {
			It("should not report error", func() {
				getTestReconciler()
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig is not ready", func() {
			It("should not do anything", func() {
				getTestReconciler(gwConfig)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig does not apply to the node", func() {
			It("should not do anything", func() {
				gwConfig.Status = getTestGwConfigStatus()
				getTestReconciler(gwConfig)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("secret is not found", func() {
			It("should report error", func() {
				gwConfig.Status = getTestGwConfigStatus()
				getTestReconciler(gwConfig)
				nodeTags = map[string]string{consts.AKSNodepoolTagKey: testNodepoolName}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(apierrors.IsNotFound(reconcileErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
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
			gwConfig = &egressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       testUID,
				},
				Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
						VmssResourceGroup:  vmssRG,
						VmssName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
				Status: getTestGwConfigStatus(),
			}
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
					OSProfile: imds.OSProfile{
						ComputerName: testNodeName,
					},
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
					Namespace: testSecretNamespace,
				},
				Data: map[string][]byte{
					consts.WireguardPrivateKeyName: []byte(privK),
				},
			}
			vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
					GatewayVMProfiles: []egressgatewayv1alpha1.GatewayVMProfile{
						{
							NodeName:    testNodeName,
							PrimaryIP:   "10.0.0.5",
							SecondaryIP: "10.0.0.6",
						},
					},
				},
			}
			gwConfig.Status = getTestGwConfigStatus()
			getTestReconciler(gwConfig, secret, vmConfig)

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
			mnl.EXPECT().AddrAdd(eth0, &netlink.Addr{IPNet: getIPNetWithActualIP(ilbIPCidr), Label: "eth0:egress"}).Return(nil)
			err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig.Status.GatewayServerProfile.Ip)
			Expect(err).To(BeNil())
		})

		It("should not add ilb ip to eth0 if it already exists", func() {
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
			mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(ilbIPCidr), Label: "eth0:egress"}}, nil)
			err := r.reconcileIlbIPOnHost(context.TODO(), gwConfig.Status.GatewayServerProfile.Ip)
			Expect(err).To(BeNil())
		})

		It("should delete ilb ip from eth0", func() {
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
			mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(ilbIPCidr), Label: "eth0:egress"}}, nil)
			mnl.EXPECT().AddrDel(eth0, &netlink.Addr{IPNet: getIPNetWithActualIP(ilbIPCidr), Label: "eth0:egress"}).Return(nil)
			err := r.reconcileIlbIPOnHost(context.TODO(), "")
			Expect(err).To(BeNil())
		})

		It("should not delete ilb ip to eth0 if it does not exist", func() {
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
			mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil)
			err := r.reconcileIlbIPOnHost(context.TODO(), "")
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
			primaryIP, secondaryIP, err := r.getVMIP(context.TODO(), gwConfig)
			Expect(err).To(BeNil())
			Expect(primaryIP).To(Equal("10.0.0.5"))
			Expect(secondaryIP).To(Equal("10.0.0.6"))
		})

		It("should remove secondary ip from eth0", func() {
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			mnl.EXPECT().LinkByName("eth0").Return(eth0, nil)
			mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(ilbIPCidr)}}, nil)
			mnl.EXPECT().AddrDel(eth0, &netlink.Addr{IPNet: getIPNetWithActualIP(ilbIPCidr)}).Return(nil)
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
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			gomock.InOrder(
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(ilbIPCidr)}}, nil),
				mnl.EXPECT().LinkByName("eth0").Return(eth0, fmt.Errorf("failed")),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).NotTo(BeNil())
			Expect(reconcileErr.Error()).To(ContainSubstring("failed"))
		})

		It("should add iptables rule when it does not exist", func() {
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			gomock.InOrder(
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(ilbIPCidr)}}, nil),
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(nil, fmt.Errorf("failed")),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).NotTo(BeNil())
			Expect(reconcileErr.Error()).To(ContainSubstring("failed"))

			expectedDump := getHostNamespaceIptablesDump("10.0.0.6")
			fipt, ok := r.IPTables.(*fakeiptables.FakeIPTables)
			Expect(ok).To(BeTrue())
			buf := bytes.NewBuffer(nil)
			Expect(fipt.SaveInto("nat", buf)).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(expectedDump))
		})

		It("should create new wireguard interface and veth pair, routes, and iptables rules", func() {
			pk, _ := wgtypes.ParseKey(privK)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
			la1.Name = "wg-6000"
			la1.Alias = testUID // gwConfig UID
			la2.Name = "host-gateway"
			wg0 := &netlink.Wireguard{LinkAttrs: la1}
			veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
			host0 := &netlink.Veth{}
			loop := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}}
			device := &wgtypes.Device{Name: "wg-6000"}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			gomock.InOrder(
				// create network namespace
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, netlink.LinkNotFoundError{}),
				// create wireguard link, wg0
				mnl.EXPECT().LinkAdd(wg0).Return(nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().LinkSetNsFd(wg0, int(gwns.Fd())).Return(nil),
				// add address to wg0
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
				mnl.EXPECT().AddrAdd(wg0, &netlink.Addr{IPNet: getIPNetWithActualIP(consts.GatewayIP)}),
				mnl.EXPECT().LinkSetUp(wg0).Return(nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mclient.EXPECT().ConfigureDevice("wg-6000", wgtypes.Config{ListenPort: to.Ptr[int](6000), PrivateKey: &pk}).Return(nil),
				mclient.EXPECT().Close().Return(nil),
				// add veth pair in host
				mnl.EXPECT().LinkByName("host-gateway").Return(veth, netlink.LinkNotFoundError{}),
				mnl.EXPECT().LinkAdd(veth).Return(nil),
				mnl.EXPECT().LinkByName("host-gateway").Return(veth, nil),
				mnl.EXPECT().LinkSetUp(veth).Return(nil),
				mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
				mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.6/32")}).Return(nil),
				mnl.EXPECT().LinkByName("host0").Return(host0, nil),
				mnl.EXPECT().LinkSetNsFd(host0, int(gwns.Fd())).Return(nil),
				// add address and routes in gw namespace
				mnl.EXPECT().LinkByName("host0").Return(host0, nil),
				mnl.EXPECT().AddrList(host0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
				mnl.EXPECT().AddrAdd(host0, &netlink.Addr{IPNet: getIPNet("10.0.0.6/32")}).Return(nil),
				mnl.EXPECT().LinkSetUp(host0).Return(nil),
				mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
				mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.5/32")}).Return(nil),
				mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{}, nil),
				mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Gw: net.ParseIP("10.0.0.5")}).Return(nil),
				mnl.EXPECT().LinkByName("lo").Return(loop, nil),
				mnl.EXPECT().LinkSetUp(loop).Return(nil),
				// setup iptables rule
			)
			err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
			Expect(err).To(BeNil())

			// verify iptables rules
			expectedDump := getGatewayNamespaceIptablesDump(6000)
			fipt, ok := r.IPTables.(*fakeiptables.FakeIPTables)
			Expect(ok).To(BeTrue())
			buf := bytes.NewBuffer(nil)
			Expect(fipt.SaveInto("nat", buf)).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(expectedDump))
		})

		It("should not change anything when setup is complete", func() {
			pk, _ := wgtypes.ParseKey(privK)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
			la1.Name = "wg-6000"
			la2.Name = "host-gateway"
			wg0 := &netlink.Wireguard{LinkAttrs: la1}
			veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
			host0 := &netlink.Veth{}
			loop := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}}
			device := &wgtypes.Device{Name: "wg-6000", ListenPort: 6000, PrivateKey: pk}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			gomock.InOrder(
				// create network namespace
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				// check address and wg config for wg0
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(consts.GatewayIP)}}, nil),
				mnl.EXPECT().LinkSetUp(wg0).Return(nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mclient.EXPECT().Close().Return(nil),
				// check veth pair in host
				mnl.EXPECT().LinkByName("host-gateway").Return(veth, nil),
				mnl.EXPECT().LinkSetUp(veth).Return(nil),
				mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.6/32")}}, nil),
				mnl.EXPECT().LinkByName("host0").Return(host0, netlink.LinkNotFoundError{}),
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
			)
			err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
			Expect(err).To(BeNil())

			// verify iptables rules
			expectedDump := getGatewayNamespaceIptablesDump(6000)
			fipt, ok := r.IPTables.(*fakeiptables.FakeIPTables)
			Expect(ok).To(BeTrue())
			buf := bytes.NewBuffer(nil)
			Expect(fipt.SaveInto("nat", buf)).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(expectedDump))
		})

		It("should delete wireguard link if any setup fails", func() {
			pk, _ := wgtypes.ParseKey(privK)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			la1, la2 := netlink.NewLinkAttrs(), netlink.NewLinkAttrs()
			la1.Name = "wg-6000"
			la1.Alias = testUID // gwConfig UID
			la2.Name = "host-gateway"
			wg0 := &netlink.Wireguard{LinkAttrs: la1}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			gomock.InOrder(
				// create network namespace
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, netlink.LinkNotFoundError{}),
				// create wireguard link, wg0
				mnl.EXPECT().LinkAdd(wg0).Return(nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().LinkSetNsFd(wg0, int(gwns.Fd())).Return(fmt.Errorf("failed")),
				mnl.EXPECT().LinkDel(wg0).Return(nil),
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
			la1.Name = "wg-6000"
			la2.Name = "host-gateway"
			wg0 := &netlink.Wireguard{LinkAttrs: la1}
			veth := &netlink.Veth{LinkAttrs: la2, PeerName: "host0"}
			device := &wgtypes.Device{Name: "wg-6000", ListenPort: 6000, PrivateKey: pk}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			gomock.InOrder(
				// get network namespace
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				// check address and wg config for wg0
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().AddrList(wg0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNetWithActualIP(consts.GatewayIP)}}, nil),
				mnl.EXPECT().LinkSetUp(wg0).Return(nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mclient.EXPECT().Close().Return(nil),
				// add veth pair in host
				mnl.EXPECT().LinkByName("host-gateway").Return(veth, netlink.LinkNotFoundError{}),
				mnl.EXPECT().LinkAdd(veth).Return(nil),
				mnl.EXPECT().LinkByName("host-gateway").Return(veth, nil),
				mnl.EXPECT().LinkSetUp(veth).Return(fmt.Errorf("failed")),
				mnl.EXPECT().LinkDel(veth).Return(nil),
			)
			err := r.configureGatewayNamespace(context.TODO(), gwConfig, &pk, "10.0.0.5", "10.0.0.6")
			Expect(errors.Unwrap(errors.Unwrap(err))).To(Equal(fmt.Errorf("failed")))
		})

		Context("Test updating gateway node status", func() {
			BeforeEach(func() {
				os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
				os.Setenv(consts.NodeNameEnvKey, testNodeName)
			})

			AfterEach(func() {
				os.Setenv(consts.PodNamespaceEnvKey, "")
				os.Setenv(consts.NodeNameEnvKey, "")
			})

			gwNamespace := egressgatewayv1alpha1.GatewayConfiguration{
				InterfaceName: "wg",
			}

			It("should create new gateway status object if not exist", func() {
				getTestReconciler(node)
				err := r.updateGatewayNodeStatus(context.TODO(), gwNamespace, PeerUpdateOpAdd)
				Expect(err).To(BeNil())
				gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
				err = getGatewayStatus(r.Client, gwStatus)
				Expect(err).To(BeNil())
				Expect(len(gwStatus.Spec.ReadyGatewayConfigurations)).To(Equal(1))
				Expect(gwStatus.Spec.ReadyGatewayConfigurations[0]).To(Equal(gwNamespace))
			})

			It("should add to existing gateway status object", func() {
				existing := &egressgatewayv1alpha1.GatewayStatus{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testNodeName,
						Namespace: testPodNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayStatusSpec{
						ReadyGatewayConfigurations: []egressgatewayv1alpha1.GatewayConfiguration{
							{
								InterfaceName: "wg1",
							},
						},
					},
				}
				getTestReconciler(node, existing)
				err := r.updateGatewayNodeStatus(context.TODO(), gwNamespace, PeerUpdateOpAdd)
				Expect(err).To(BeNil())
				gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
				err = getGatewayStatus(r.Client, gwStatus)
				Expect(err).To(BeNil())
				var namespaces []string
				for _, peer := range gwStatus.Spec.ReadyGatewayConfigurations {
					namespaces = append(namespaces, peer.InterfaceName)
				}
				sort.Strings(namespaces)
				Expect(namespaces).To(Equal([]string{"wg", "wg1"}))
			})

			It("should remove from existing gateway status object", func() {
				existing := &egressgatewayv1alpha1.GatewayStatus{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testNodeName,
						Namespace: testPodNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayStatusSpec{
						ReadyGatewayConfigurations: []egressgatewayv1alpha1.GatewayConfiguration{
							{
								InterfaceName: "wg",
							},
							{
								InterfaceName: "wg1",
							},
						},
						ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
							{
								InterfaceName: "wg",
								PublicKey:     "pubk1",
							},
							{
								InterfaceName: "wg1",
								PublicKey:     "pubk2",
							},
							{
								InterfaceName: "wg",
								PublicKey:     "pubk3",
							},
						},
					},
				}
				getTestReconciler(node, existing)
				err := r.updateGatewayNodeStatus(context.TODO(), gwNamespace, PeerUpdateOpDelete)
				Expect(err).To(BeNil())
				gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
				err = getGatewayStatus(r.Client, gwStatus)
				Expect(err).To(BeNil())
				Expect(len(gwStatus.Spec.ReadyGatewayConfigurations)).To(Equal(1))
				Expect(len(gwStatus.Spec.ReadyPeerConfigurations)).To(Equal(1))
				Expect(gwStatus.Spec.ReadyGatewayConfigurations[0].InterfaceName).To(Equal("wg1"))
				Expect(gwStatus.Spec.ReadyPeerConfigurations[0].InterfaceName).To(Equal("wg1"))
			})
		})
	})

	Context("Test reconcile deletion", func() {
		var vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration
		var gwStatus *egressgatewayv1alpha1.GatewayStatus
		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "",
					Namespace: "",
				},
			}
			gwConfig = &egressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       testUID,
				},
				Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
						VmssResourceGroup:  vmssRG,
						VmssName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
				Status: getTestGwConfigStatus(),
			}
			vmConfig = &egressgatewayv1alpha1.GatewayVMConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
					GatewayVMProfiles: []egressgatewayv1alpha1.GatewayVMProfile{
						{
							NodeName:    testNodeName,
							PrimaryIP:   "10.0.0.5",
							SecondaryIP: "10.0.0.6",
						},
					},
				},
			}
			gwStatus = &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNodeName,
					Namespace: testPodNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyGatewayConfigurations: []egressgatewayv1alpha1.GatewayConfiguration{
						{
							InterfaceName: "wg-6000",
						},
						{
							InterfaceName: "wg-6001",
						},
					},
					ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
						{
							InterfaceName: "wg-6001",
							PublicKey:     "pubk1",
						},
						{
							InterfaceName: "wg-6001",
							PublicKey:     "pubk2",
						},
						{
							InterfaceName: "wg-6000",
							PublicKey:     "pubk3",
						},
					},
				},
			}
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
					OSProfile: imds.OSProfile{
						ComputerName: testNodeName,
					},
				},
				Network: &imds.NetworkMetadata{
					Interface: []imds.NetworkInterface{
						{IPv4: imds.IPData{Subnet: []imds.Subnet{{Prefix: "31"}}}},
					},
				},
			}
			os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			os.Setenv(consts.NodeNameEnvKey, testNodeName)
		})

		AfterEach(func() {
			os.Setenv(consts.PodNamespaceEnvKey, "")
			os.Setenv(consts.NodeNameEnvKey, "")
		})

		It("should delete wglink, vmSecondaryIP, iptables rules and update gwStatus, while not delete ilb IP", func() {
			getTestReconciler(node, gwConfig, vmConfig, gwStatus)
			// existing gwConfig takes wglink wg-6000 and vmSecondaryIP 10.0.0.6, other links and ips should be deleted
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			host0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "host0"}}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			linkToDel := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6001", Alias: "deletingUID"}}
			routeToDel := netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.7/32")}

			// create existing iptables rules first
			existingHostDump := getHostNamespaceIptablesDump("10.0.0.6", "10.0.0.7")
			existingGWDump := getGatewayNamespaceIptablesDump(6000, 6001)
			fipt, ok := r.IPTables.(*fakeiptables.FakeIPTables)
			Expect(ok).To(BeTrue())
			Expect(fipt.RestoreAll([]byte(existingHostDump), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters)).NotTo(HaveOccurred())
			Expect(fipt.RestoreAll([]byte(existingGWDump), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters)).NotTo(HaveOccurred())

			// add ActiveGateways
			Expect(r.LBProbeServer.AddGateway("deletingUID")).To(Succeed())
			Expect(r.LBProbeServer.AddGateway("notDeletingUID")).To(Succeed())

			gomock.InOrder(
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{
					&netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "wg-6000"}},
					&netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "host0"}},
					&netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}},
					linkToDel,
				}, nil),
				mnl.EXPECT().LinkByName("host0").Return(host0, nil),
				mnl.EXPECT().AddrList(host0, nl.FAMILY_ALL).Return([]netlink.Addr{{IPNet: getIPNet("10.0.0.6/32")}, {IPNet: getIPNet("10.0.0.7/32")}}, nil),
				mnl.EXPECT().LinkByName("host0").Return(host0, nil),
				mnl.EXPECT().AddrDel(host0, &netlink.Addr{IPNet: getIPNet("10.0.0.7/32")}).Return(nil),
				mnl.EXPECT().RouteList(nil, nl.FAMILY_ALL).Return([]netlink.Route{
					{LinkIndex: 0, Scope: netlink.SCOPE_UNIVERSE, Dst: getIPNet("10.0.0.6/32")},
					routeToDel,
				}, nil),
				mnl.EXPECT().RouteDel(&routeToDel).Return(nil),
				mnl.EXPECT().LinkDel(linkToDel).Return(nil),
			)
			res, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
			Expect(res).To(Equal(ctrl.Result{}))
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
			err := getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			Expect(len(gwStatus.Spec.ReadyGatewayConfigurations)).To(Equal(1))
			Expect(len(gwStatus.Spec.ReadyPeerConfigurations)).To(Equal(1))

			tmpipt := fakeiptables.NewFake()
			leftHostDump := getHostNamespaceIptablesDump("10.0.0.6")
			leftGWDump := getGatewayNamespaceIptablesDump(6000)
			Expect(tmpipt.RestoreAll([]byte(leftHostDump), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters)).NotTo(HaveOccurred())
			Expect(tmpipt.RestoreAll([]byte(leftGWDump), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters)).NotTo(HaveOccurred())
			expectedBuf := bytes.NewBuffer(nil)
			Expect(tmpipt.SaveInto("nat", expectedBuf)).NotTo(HaveOccurred())
			existingBuf := bytes.NewBuffer(nil)
			Expect(fipt.SaveInto("nat", existingBuf)).NotTo(HaveOccurred())
			Expect(existingBuf.String()).To(Equal(expectedBuf.String()))

			Expect(r.LBProbeServer.GetGateways()).To(Equal([]string{"notDeletingUID"}))
		})

		It("should do fully cleanup when there's no active gwConfig", func() {
			gwConfig.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}
			controllerutil.AddFinalizer(gwConfig, consts.SGCFinalizerName)
			getTestReconciler(node, gwConfig, vmConfig, gwStatus)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			eth0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "eth0"}}
			host0 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "host0"}}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			linkToDel := netlink.Addr{IPNet: getIPNetWithActualIP(ilbIPCidr), Label: "eth0:egress"}

			existingHostDump := getHostNamespaceIptablesDump()
			fipt, ok := r.IPTables.(*fakeiptables.FakeIPTables)
			Expect(ok).To(BeTrue())
			Expect(fipt.RestoreAll([]byte(existingHostDump), utiliptables.NoFlushTables, utiliptables.NoRestoreCounters)).NotTo(HaveOccurred())

			gomock.InOrder(
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{
					&netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "host0"}},
					&netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "lo"}},
				}, nil),
				mnl.EXPECT().LinkByName("host0").Return(host0, nil),
				mnl.EXPECT().AddrList(host0, nl.FAMILY_ALL).Return([]netlink.Addr{}, nil),
				mnl.EXPECT().LinkByName("eth0").Return(eth0, nil),
				mnl.EXPECT().AddrList(eth0, nl.FAMILY_ALL).Return([]netlink.Addr{linkToDel}, nil),
				mnl.EXPECT().AddrDel(eth0, &linkToDel).Return(nil),
			)
			res, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
			Expect(res).To(Equal(ctrl.Result{}))

			expectedDump := natBuiltinChains + `
COMMIT
`
			buf := bytes.NewBuffer(nil)
			Expect(fipt.SaveInto("nat", buf)).NotTo(HaveOccurred())
			Expect(buf.String()).To(Equal(expectedDump))
		})
	})
})

func getTestGwConfigStatus() egressgatewayv1alpha1.StaticGatewayConfigurationStatus {
	return egressgatewayv1alpha1.StaticGatewayConfigurationStatus{
		EgressIpPrefix: "1.2.3.4/31",
		GatewayServerProfile: egressgatewayv1alpha1.GatewayServerProfile{
			Ip:        ilbIP,
			Port:      6000,
			PublicKey: pubK,
			PrivateKeySecretRef: &corev1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       testName,
				Namespace:  testSecretNamespace,
			},
		},
	}
}

func getIPNet(ipCidr string) *net.IPNet {
	_, ipNet, _ := net.ParseCIDR(ipCidr)
	return ipNet
}

func getIPNetWithActualIP(ipCidr string) *net.IPNet {
	ipNet, _ := netlink.ParseIPNet(ipCidr)
	return ipNet
}

func getHostNamespaceIptablesDump(ips ...string) string {
	res := natBuiltinChains + `
:` + `EGRESS-GATEWAY-SNAT` + ` - [0:0]
`
	for _, ip := range ips {
		res += `:` + `EGRESS-` + strings.ReplaceAll(ip, ".", "-") + ` - [0:0]
`
	}
	res += `-A ` + string(utiliptables.ChainPostrouting) + ` -m comment --comment kube-egress-gateway no MASQUERADE -j EGRESS-GATEWAY-SNAT
`
	for _, ip := range ips {
		res += `-A ` + `EGRESS-GATEWAY-SNAT` + ` -m comment --comment kube-egress-gateway no sNAT packet from ip ` + ip + ` -j EGRESS-` + strings.ReplaceAll(ip, ".", "-") + `
`
	}
	for _, ip := range ips {
		res += `-A ` + `EGRESS-` + strings.ReplaceAll(ip, ".", "-") + ` -s ` + ip + `/32 -j ACCEPT
`
	}
	res += `COMMIT
`
	return res
}

func getGatewayNamespaceIptablesDump(marks ...int) string {
	res := natBuiltinChains + `
`
	for _, mark := range marks {
		res += `:` + `EGRESS-GATEWAY-MARK-` + strconv.Itoa(mark) + ` - [0:0]
:` + `EGRESS-GATEWAY-SNAT-` + strconv.Itoa(mark) + ` - [0:0]
`
	}
	for _, mark := range marks {
		res += `-A ` + string(utiliptables.ChainPrerouting) + ` -m comment --comment kube-egress-gateway mark packets from gateway link wg-` + strconv.Itoa(mark) + ` -j EGRESS-GATEWAY-MARK-` + strconv.Itoa(mark) + `
-A ` + string(utiliptables.ChainPostrouting) + ` -m comment --comment kube-egress-gateway sNAT packets from gateway link wg-` + strconv.Itoa(mark) + ` -j EGRESS-GATEWAY-SNAT-` + strconv.Itoa(mark) + `
`
	}
	for _, mark := range marks {
		res += `-A ` + `EGRESS-GATEWAY-MARK-` + strconv.Itoa(mark) + ` -i wg-` + strconv.Itoa(mark) + ` -j CONNMARK --set-mark ` + strconv.Itoa(mark) + `
-A ` + `EGRESS-GATEWAY-SNAT-` + strconv.Itoa(mark) + ` -o host0 -m connmark --mark ` + strconv.Itoa(mark) + ` -j SNAT --to-source 10.0.0.6
`
	}
	res += `COMMIT
`
	return res
}
