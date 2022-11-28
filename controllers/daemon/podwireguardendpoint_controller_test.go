package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper/mocknetlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper/mockwgctrlwrapper"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	pubK2  = "xUgp0rzI2lqa78w9vRTfCTx8UQzZacu4WXXKw86Oy0c="
	privK2 = "OGDxE0+PqdflLqQxdlHigfC7ZKtEh2VMxIElq4RpZWc="
)

var _ = Describe("Daemon PodWireguardEndpoint controller unit tests", func() {
	var (
		r            *PodWireguardEndpointReconciler
		req          reconcile.Request
		res          reconcile.Result
		reconcileErr error
		podEndpoint  *kubeegressgatewayv1alpha1.PodWireguardEndpoint
		gwConfig     *kubeegressgatewayv1alpha1.StaticGatewayConfiguration
		mclient      *mockwgctrlwrapper.MockClient
	)

	getTestReconciler := func(objects ...runtime.Object) {
		mctrl := gomock.NewController(GinkgoT())
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objects...).Build()
		r = &PodWireguardEndpointReconciler{Client: cl}
		r.Netlink = mocknetlinkwrapper.NewMockInterface(mctrl)
		r.NetNS = mocknetnswrapper.NewMockInterface(mctrl)
		r.WgCtrl = mockwgctrlwrapper.NewMockInterface(mctrl)
		mclient = mockwgctrlwrapper.NewMockClient(mctrl)
	}

	Context("Reconcile", func() {
		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			podEndpoint = &kubeegressgatewayv1alpha1.PodWireguardEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: kubeegressgatewayv1alpha1.PodWireguardEndpointSpec{
					StaticGatewayConfiguration: testName,
					PodIpAddress:               "10.0.0.25",
					PodWireguardPublicKey:      pubK,
				},
			}
			gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       "1234567890",
				},
				Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  vmssRG,
						VMSSName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
			}
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName + "a",
					ResourceGroupName: vmssRG,
				},
			}
		})

		When("gwConfig is not found", func() {
			It("should report error", func() {
				getTestReconciler(podEndpoint)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(apierrors.IsNotFound(reconcileErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig does not apply to the node", func() {
			It("should not do anything", func() {
				getTestReconciler(podEndpoint, gwConfig)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
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
			podEndpoint = &kubeegressgatewayv1alpha1.PodWireguardEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: kubeegressgatewayv1alpha1.PodWireguardEndpointSpec{
					StaticGatewayConfiguration: testName,
					PodIpAddress:               "10.0.0.25",
					PodWireguardPublicKey:      pubK,
				},
			}
			gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       "1234567890",
				},
				Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  vmssRG,
						VMSSName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
			}
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
				},
			}
			getTestReconciler(podEndpoint, gwConfig)
		})

		It("should report error when gateway namespace is not found", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			gomock.InOrder(
				mns.EXPECT().GetNS(nsName).Return(nil, os.ErrNotExist),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(os.ErrNotExist))
		})

		It("should report error when failed to create wgCtrl client", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
			gomock.InOrder(
				mns.EXPECT().GetNS(nsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(nil, fmt.Errorf("failed")),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
		})

		It("should report error when failed to configure wireguard device", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
			pk, _ := wgtypes.ParseKey(pubK)
			config := wgtypes.Config{
				Peers: []wgtypes.PeerConfig{
					{
						PublicKey:         pk,
						ReplaceAllowedIPs: true,
						AllowedIPs: []net.IPNet{
							*getIPNet("10.0.0.25/32"),
						},
					},
				},
			}
			gomock.InOrder(
				mns.EXPECT().GetNS(nsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().ConfigureDevice("wg0", config).Return(fmt.Errorf("failed")),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
		})

		Context("when adding peer route", func() {
			var (
				mns  *mocknetnswrapper.MockInterface
				gwns *mocknetnswrapper.MockNetNS
			)
			BeforeEach(func() {
				mns = r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				gwns = &mocknetnswrapper.MockNetNS{Name: nsName}
				pk, _ := wgtypes.ParseKey(pubK)
				config := wgtypes.Config{
					Peers: []wgtypes.PeerConfig{
						{
							PublicKey:         pk,
							ReplaceAllowedIPs: true,
							AllowedIPs: []net.IPNet{
								*getIPNet("10.0.0.25/32"),
							},
						},
					},
				}
				gomock.InOrder(
					mns.EXPECT().GetNS(nsName).Return(gwns, nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mclient.EXPECT().ConfigureDevice("wg0", config).Return(nil),
					mclient.EXPECT().Close().Return(nil),
				)
			})

			It("should report error if failed to get wireguard link", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				wg0 := &netlink.Wireguard{}
				mnl.EXPECT().LinkByName("wg0").Return(wg0, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(errors.Unwrap(reconcileErr))).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error if failed to add route", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				wg0 := &netlink.Wireguard{}
				gomock.InOrder(
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.25/32")}).Return(fmt.Errorf("failed")),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(errors.Unwrap(reconcileErr))).To(Equal(fmt.Errorf("failed")))
			})

			It("should succeed", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				wg0 := &netlink.Wireguard{}
				gomock.InOrder(
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet("10.0.0.25/32")}).Return(nil),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
			})
		})
	})

	Context("Test reconcile peerConfig cleanup", func() {
		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "",
					Namespace: "",
				},
			}
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
				},
			}
		})

		It("should clean deleted peer and route", func() {
			gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       "1234567890",
				},
				Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  vmssRG,
						VMSSName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
			}
			getTestReconciler(gwConfig)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			wg0 := &netlink.Wireguard{}
			gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
			pk, _ := wgtypes.ParseKey(pubK)
			device := &wgtypes.Device{
				Peers: []wgtypes.Peer{
					{
						PublicKey: pk,
						AllowedIPs: []net.IPNet{
							*getIPNet("10.0.0.1/32"),
							*getIPNet("10.0.0.2/32"),
						},
					},
				},
			}
			config := wgtypes.Config{
				Peers: []wgtypes.PeerConfig{
					{
						PublicKey: pk,
						Remove:    true,
					},
				},
			}
			gomock.InOrder(
				mns.EXPECT().GetNS(nsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg0").Return(device, nil),
				mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
				mnl.EXPECT().RouteList(wg0, netlink.FAMILY_ALL).Return([]netlink.Route{{Dst: getIPNet("10.0.0.1/32")}}, nil),
				mnl.EXPECT().RouteDel(&netlink.Route{Dst: getIPNet("10.0.0.1/32")}).Return(nil),
				mclient.EXPECT().ConfigureDevice("wg0", config).Return(nil),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})

		It("should not clean existing peer and route", func() {
			podEndpoint = &kubeegressgatewayv1alpha1.PodWireguardEndpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName + "a",
					Namespace: testNamespace,
				},
				Spec: kubeegressgatewayv1alpha1.PodWireguardEndpointSpec{
					StaticGatewayConfiguration: testName,
					PodIpAddress:               "10.0.0.1",
					PodWireguardPublicKey:      pubK,
				},
			}
			gwConfig = &kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       "1234567890",
				},
				Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
					GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  vmssRG,
						VMSSName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
			}
			getTestReconciler(podEndpoint, gwConfig)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
			pk, _ := wgtypes.ParseKey(pubK)
			device := &wgtypes.Device{
				Peers: []wgtypes.Peer{
					{
						PublicKey: pk,
						AllowedIPs: []net.IPNet{
							*getIPNet("10.0.0.1/32"),
						},
					},
				},
			}
			gomock.InOrder(
				mns.EXPECT().GetNS(nsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg0").Return(device, nil),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})

		It("should handle multiple gateway namespaces properly", func() {
			objects := []runtime.Object{
				&kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
						UID:       "1234567890",
					},
					Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
						GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  vmssRG,
							VMSSName:           vmssName,
							PublicIpPrefixSize: 31,
						},
					},
				},
				&kubeegressgatewayv1alpha1.StaticGatewayConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName + "a",
						Namespace: testNamespace,
						UID:       "1234567891",
					},
					Spec: kubeegressgatewayv1alpha1.StaticGatewayConfigurationSpec{
						GatewayVMSSProfile: kubeegressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  vmssRG,
							VMSSName:           vmssName,
							PublicIpPrefixSize: 31,
						},
					},
				},
				&kubeegressgatewayv1alpha1.PodWireguardEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName + "a",
						Namespace: testNamespace,
					},
					Spec: kubeegressgatewayv1alpha1.PodWireguardEndpointSpec{
						StaticGatewayConfiguration: testName,
						PodIpAddress:               "10.0.0.1",
						PodWireguardPublicKey:      pubK,
					},
				},
			}
			getTestReconciler(objects...)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			wg0 := &netlink.Wireguard{}
			gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
			pk, _ := wgtypes.ParseKey(pubK)
			pk2, _ := wgtypes.ParseKey(pubK2)
			device := &wgtypes.Device{
				Peers: []wgtypes.Peer{
					{
						PublicKey: pk,
						AllowedIPs: []net.IPNet{
							*getIPNet("10.0.0.1/32"),
						},
					},
					{
						PublicKey: pk2,
						AllowedIPs: []net.IPNet{
							*getIPNet("10.0.0.2/32"),
						},
					},
				},
			}
			config := wgtypes.Config{
				Peers: []wgtypes.PeerConfig{
					{
						PublicKey: pk2,
						Remove:    true,
					},
				},
			}
			// 1st gateway namespace, delete one peer and keey one peer
			mns.EXPECT().GetNS(nsName).Return(gwns, nil)
			mwg.EXPECT().New().Return(mclient, nil)
			mclient.EXPECT().Device("wg0").Return(device, nil)
			mnl.EXPECT().LinkByName("wg0").Return(wg0, nil)
			mnl.EXPECT().RouteList(wg0, netlink.FAMILY_ALL).Return([]netlink.Route{{Dst: getIPNet("10.0.0.1/32")}, {Dst: getIPNet("10.0.0.2/32")}}, nil)
			mnl.EXPECT().RouteDel(&netlink.Route{Dst: getIPNet("10.0.0.2/32")}).Return(nil)
			mclient.EXPECT().ConfigureDevice("wg0", config).Return(nil)
			mclient.EXPECT().Close().Return(nil)
			// 2nd gateway namespace, return error, should not block
			mns.EXPECT().GetNS(nsName[:len(nsName)-1]+"1").Return(nil, fmt.Errorf("failed"))
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})
	})
})
