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
	"errors"
	"fmt"
	"net"
	"os"
	"sort"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/imds"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper/mocknetlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper/mockwgctrlwrapper"
)

const (
	pubK2        = "xUgp0rzI2lqa78w9vRTfCTx8UQzZacu4WXXKw86Oy0c="
	privK2       = "OGDxE0+PqdflLqQxdlHigfC7ZKtEh2VMxIElq4RpZWc="
	podIPAddrNet = "10.0.0.25/32"
)

var _ = Describe("Daemon PodWireguardEndpoint controller unit tests", func() {
	var (
		r            *PodWireguardEndpointReconciler
		req          reconcile.Request
		res          reconcile.Result
		reconcileErr error
		podEndpoint  *egressgatewayv1alpha1.PodWireguardEndpoint
		gwConfig     *egressgatewayv1alpha1.StaticGatewayConfiguration
		mclient      *mockwgctrlwrapper.MockClient
		node         = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNodeName}}
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

	getTestPodEndpoint := func() *egressgatewayv1alpha1.PodWireguardEndpoint {
		return &egressgatewayv1alpha1.PodWireguardEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: egressgatewayv1alpha1.PodWireguardEndpointSpec{
				StaticGatewayConfiguration: testName,
				PodIpAddress:               podIPAddrNet,
				PodWireguardPublicKey:      pubK,
			},
		}
	}

	getTestGwConfig := func() *egressgatewayv1alpha1.StaticGatewayConfiguration {
		return &egressgatewayv1alpha1.StaticGatewayConfiguration{
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
	}

	Context("Skip reconcile", func() {
		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			podEndpoint = getTestPodEndpoint()
			gwConfig = getTestGwConfig()
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
			podEndpoint = getTestPodEndpoint()
			gwConfig = getTestGwConfig()
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
				},
			}
			os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			os.Setenv(consts.NodeNameEnvKey, testNodeName)
			getTestReconciler(podEndpoint, gwConfig, node)
		})

		AfterEach(func() {
			os.Setenv(consts.PodNamespaceEnvKey, "")
			os.Setenv(consts.NodeNameEnvKey, "")
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
							*getIPNet(podIPAddrNet),
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

		Context("test adding peer route", func() {
			BeforeEach(func() {
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
								*getIPNet(podIPAddrNet),
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
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(fmt.Errorf("failed")),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(errors.Unwrap(reconcileErr))).To(Equal(fmt.Errorf("failed")))
			})

			It("should succeed and update gateway status", func() {
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				wg0 := &netlink.Wireguard{}
				gomock.InOrder(
					mnl.EXPECT().LinkByName("wg0").Return(wg0, nil),
					mnl.EXPECT().RouteReplace(&netlink.Route{LinkIndex: 0, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(nil),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
				err := getGatewayStatus(r.Client, gwStatus)
				Expect(err).To(BeNil())
				Expect(gwStatus.Spec.ReadyPeerConfigurations).To(Equal([]egressgatewayv1alpha1.PeerConfiguration{
					{
						PublicKey:            pubK,
						NetnsName:            nsName,
						PodWireguardEndpoint: fmt.Sprintf("%s/%s", testNamespace, testName),
					},
				}))
			})
		})
	})

	Context("Test updating gateway node status", func() {
		peerConfigs := []egressgatewayv1alpha1.PeerConfiguration{
			{
				PublicKey: "pubk1",
				NetnsName: "ns1",
			},
			{
				PublicKey: "pubk2",
				NetnsName: "ns2",
			},
		}

		BeforeEach(func() {
			os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			os.Setenv(consts.NodeNameEnvKey, testNodeName)
		})

		AfterEach(func() {
			os.Setenv(consts.PodNamespaceEnvKey, "")
			os.Setenv(consts.NodeNameEnvKey, "")
		})

		It("should create new gateway status object if not exist", func() {
			getTestReconciler(node)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, true)
			Expect(err).To(BeNil())
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
			err = getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			Expect(gwStatus.Spec.ReadyPeerConfigurations).To(Equal(peerConfigs))
		})

		It("should update existing gateway status object", func() {
			existing := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNodeName,
					Namespace: testPodNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
						{
							PublicKey: "pubk1",
							NetnsName: "ns1",
						},
						{
							PublicKey: "pubk3",
							NetnsName: "ns3",
						},
					},
				},
			}
			getTestReconciler(node, existing)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, true)
			Expect(err).To(BeNil())
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
			err = getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			var keys []string
			for _, peer := range gwStatus.Spec.ReadyPeerConfigurations {
				keys = append(keys, peer.PublicKey)
			}
			sort.Strings(keys)
			Expect(keys).To(Equal([]string{"pubk1", "pubk2", "pubk3"}))
		})

		It("should update existing gateway status object - deletion", func() {
			existing := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNodeName,
					Namespace: testPodNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
						{
							PublicKey: "pubk1",
							NetnsName: "ns1",
						},
						{
							PublicKey: "pubk3",
							NetnsName: "ns3",
						},
					},
				},
			}
			getTestReconciler(node, existing)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, false)
			Expect(err).To(BeNil())
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
			err = getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			Expect(len(gwStatus.Spec.ReadyPeerConfigurations)).To(Equal(1))
			Expect(gwStatus.Spec.ReadyPeerConfigurations[0].PublicKey).To(Equal("pubk3"))
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

			os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			os.Setenv(consts.NodeNameEnvKey, testNodeName)
		})

		AfterEach(func() {
			os.Setenv(consts.PodNamespaceEnvKey, "")
			os.Setenv(consts.NodeNameEnvKey, "")
		})

		It("should clean deleted peer and route", func() {
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNodeName,
					Namespace: testPodNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
						{
							PublicKey: pubK,
							NetnsName: "ns1",
						},
					},
				},
			}
			gwConfig = getTestGwConfig()
			getTestReconciler(gwConfig, gwStatus)
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
			err := getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			Expect(gwStatus.Spec.ReadyPeerConfigurations).To(BeEmpty())
		})

		It("should not clean existing peer and route", func() {
			podEndpoint = getTestPodEndpoint()
			podEndpoint.Name = testName + "a"
			gwConfig = getTestGwConfig()
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
				getTestGwConfig(),
				&egressgatewayv1alpha1.StaticGatewayConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName + "a",
						Namespace: testNamespace,
						UID:       "1234567891",
					},
					Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
						GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
							VmssResourceGroup:  vmssRG,
							VmssName:           vmssName,
							PublicIpPrefixSize: 31,
						},
					},
					Status: getTestGwConfigStatus(),
				},
				&egressgatewayv1alpha1.PodWireguardEndpoint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName + "a",
						Namespace: testNamespace,
					},
					Spec: egressgatewayv1alpha1.PodWireguardEndpointSpec{
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
			mns.EXPECT().GetNS("gw-1234567891-10_0_0_4").Return(nil, fmt.Errorf("failed"))
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})
	})
})

func getGatewayStatus(cl client.Client, gwStatus *egressgatewayv1alpha1.GatewayStatus) error {
	key := types.NamespacedName{
		Name:      testNodeName,
		Namespace: testPodNamespace,
	}
	err := cl.Get(context.TODO(), key, gwStatus)
	return err
}
