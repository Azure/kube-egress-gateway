// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
	"go.uber.org/mock/gomock"
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
	podIPAddress = "10.0.0.25"
	podIPAddrNet = "10.0.0.25/32"
	podUID       = "test-pod-uid"
)

var _ = Describe("Daemon PodEndpoint controller unit tests", func() {
	var (
		r            *PodEndpointReconciler
		req          reconcile.Request
		res          reconcile.Result
		reconcileErr error
		podEndpoint  *egressgatewayv1alpha1.PodEndpoint
		pod          *corev1.Pod
		gwConfig     *egressgatewayv1alpha1.StaticGatewayConfiguration
		mclient      *mockwgctrlwrapper.MockClient
		node         = &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: testNodeName}}
	)

	getTestReconciler := func(objects ...runtime.Object) {
		mctrl := gomock.NewController(GinkgoT())
		cl := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(objects...).Build()
		r = &PodEndpointReconciler{Client: cl}
		r.Netlink = mocknetlinkwrapper.NewMockInterface(mctrl)
		r.NetNS = mocknetnswrapper.NewMockInterface(mctrl)
		r.WgCtrl = mockwgctrlwrapper.NewMockInterface(mctrl)
		mclient = mockwgctrlwrapper.NewMockClient(mctrl)
	}

	getTestPodEndpoint := func() *egressgatewayv1alpha1.PodEndpoint {
		controller := true
		blockOwnerDeletion := true
		return &egressgatewayv1alpha1.PodEndpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:       testName,
				Namespace:  testNamespace,
				Generation: 1,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         corev1.SchemeGroupVersion.String(),
						Kind:               "Pod",
						Name:               testName,
						UID:                podUID,
						Controller:         &controller,
						BlockOwnerDeletion: &blockOwnerDeletion,
					},
				},
			},
			Spec: egressgatewayv1alpha1.PodEndpointSpec{
				StaticGatewayConfiguration: testName,
				PodIpAddress:               podIPAddrNet,
				PodPublicKey:               pubK,
			},
			Status: egressgatewayv1alpha1.PodEndpointStatus{
				PodUID:             podUID,
				ObservedGeneration: 1,
			},
		}
	}

	getTestPod := func() *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
				UID:       podUID,
			},
			Status: corev1.PodStatus{
				PodIP: podIPAddress,
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
			pod = getTestPod()
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
				getTestReconciler(podEndpoint, pod)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(apierrors.IsNotFound(reconcileErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("gwConfig does not apply to the node", func() {
			It("should not do anything", func() {
				getTestReconciler(podEndpoint, pod, gwConfig)
				res, reconcileErr = r.Reconcile(context.TODO(), req)

				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})
	})

	Context("Validate PodEndpoint", func() {
		BeforeEach(func() {
			podEndpoint = getTestPodEndpoint()
			pod = getTestPod()
		})

		It("should accept a CNI-attested endpoint for the live Pod", func() {
			getTestReconciler(pod)
			Expect(r.validatePodEndpoint(context.TODO(), podEndpoint)).To(Succeed())
		})

		It("should accept the CNI-attested IP while Pod status is not populated", func() {
			pod.Status.PodIP = ""
			getTestReconciler(pod)
			Expect(r.validatePodEndpoint(context.TODO(), podEndpoint)).To(Succeed())
		})

		It("should reject an unattested spec generation", func() {
			podEndpoint.Generation++
			getTestReconciler(pod)
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should reject a changed owner reference", func() {
			podEndpoint.OwnerReferences[0].UID = "different-pod-uid"
			getTestReconciler(pod)
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should reject a recreated Pod", func() {
			pod.UID = "replacement-pod-uid"
			getTestReconciler(pod)
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should reject a missing Pod", func() {
			getTestReconciler()
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should reject a terminating Pod", func() {
			now := metav1.Now()
			pod.DeletionTimestamp = &now
			pod.Finalizers = []string{"test-finalizer"}
			getTestReconciler(pod)
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should reject an IP not assigned to the Pod", func() {
			pod.Status.PodIP = "10.0.0.26"
			getTestReconciler(pod)
			Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
		})

		It("should accept an IPv4 address from PodIPs", func() {
			pod.Status.PodIP = "2001:db8::1"
			pod.Status.PodIPs = []corev1.PodIP{
				{IP: "2001:db8::1"},
				{IP: podIPAddress},
			}
			getTestReconciler(pod)
			Expect(r.validatePodEndpoint(context.TODO(), podEndpoint)).To(Succeed())
		})

		It("should reject an IP claimed by another valid PodEndpoint", func() {
			otherEndpoint := getTestPodEndpoint()
			otherEndpoint.Name = testName + "-other"
			otherEndpoint.Status.PodUID = podUID + "-other"
			otherEndpoint.OwnerReferences[0].Name = otherEndpoint.Name
			otherEndpoint.OwnerReferences[0].UID = otherEndpoint.Status.PodUID
			otherPod := getTestPod()
			otherPod.Name = otherEndpoint.Name
			otherPod.UID = otherEndpoint.Status.PodUID
			getTestReconciler(podEndpoint, pod, otherEndpoint, otherPod)

			err := r.validateUniquePodIPClaim(context.TODO(), podEndpoint)
			Expect(errors.Is(err, errInvalidPodEndpoint)).To(BeTrue())
		})

		DescribeTable("should reject non-canonical or non-IPv4 host CIDRs",
			func(podIP string) {
				podEndpoint.Spec.PodIpAddress = podIP
				getTestReconciler(pod)
				Expect(errors.Is(r.validatePodEndpoint(context.TODO(), podEndpoint), errInvalidPodEndpoint)).To(BeTrue())
			},
			Entry("plain address", podIPAddress),
			Entry("IPv4 network", "10.0.0.0/24"),
			Entry("non-canonical host address", "10.0.0.25/032"),
			Entry("IPv6 host", "2001:db8::1/128"),
		)
	})

	Context("Ensure WireGuard peer route", func() {
		var (
			mnl        *mocknetlinkwrapper.MockInterface
			targetLink *netlink.Wireguard
			target     *net.IPNet
		)

		BeforeEach(func() {
			getTestReconciler()
			mnl = r.Netlink.(*mocknetlinkwrapper.MockInterface)
			targetLink = &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6000", Index: 10}}
			target = getIPNet(podIPAddrNet)
			gwConfig = getTestGwConfig()
		})

		It("should reuse an identical route already owned by the target link", func() {
			route := netlink.Route{LinkIndex: targetLink.Index, Scope: netlink.SCOPE_LINK, Dst: target}
			gomock.InOrder(
				mnl.EXPECT().LinkByName("wg-6000").Return(targetLink, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{targetLink}, nil),
				mnl.EXPECT().RouteList(targetLink, netlink.FAMILY_V4).Return([]netlink.Route{route}, nil),
			)

			found, created, err := r.ensureWireguardPeerRoute(gwConfig, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
			Expect(found).To(Equal(&route))
		})

		It("should reject a route overlapping another WireGuard link", func() {
			otherLink := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6001", Index: 11}}
			overlap := getIPNet("10.0.0.0/24")
			gomock.InOrder(
				mnl.EXPECT().LinkByName("wg-6000").Return(targetLink, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{targetLink, otherLink}, nil),
				mnl.EXPECT().RouteList(targetLink, netlink.FAMILY_V4).Return(nil, nil),
				mnl.EXPECT().RouteList(otherLink, netlink.FAMILY_V4).Return([]netlink.Route{
					{LinkIndex: otherLink.Index, Scope: netlink.SCOPE_LINK, Dst: overlap},
				}, nil),
			)

			_, _, err := r.ensureWireguardPeerRoute(gwConfig, target)
			Expect(err).To(MatchError(ContainSubstring("overlaps claimed route")))
		})

		It("should tolerate an atomic add race when the winner installed the same route", func() {
			route := &netlink.Route{LinkIndex: targetLink.Index, Scope: netlink.SCOPE_LINK, Dst: target}
			gomock.InOrder(
				mnl.EXPECT().LinkByName("wg-6000").Return(targetLink, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{targetLink}, nil),
				mnl.EXPECT().RouteList(targetLink, netlink.FAMILY_V4).Return(nil, nil),
				mnl.EXPECT().RouteAdd(route).Return(syscall.EEXIST),
				mnl.EXPECT().LinkList().Return([]netlink.Link{targetLink}, nil),
				mnl.EXPECT().RouteList(targetLink, netlink.FAMILY_V4).Return([]netlink.Route{*route}, nil),
			)

			found, created, err := r.ensureWireguardPeerRoute(gwConfig, target)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
			Expect(found).To(Equal(route))
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
			pod = getTestPod()
			gwConfig = getTestGwConfig()
			nodeMeta = &imds.InstanceMetadata{
				Compute: &imds.ComputeMetadata{
					VMScaleSetName:    vmssName,
					ResourceGroupName: vmssRG,
				},
			}
			_ = os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			_ = os.Setenv(consts.NodeNameEnvKey, testNodeName)
			getTestReconciler(podEndpoint, pod, gwConfig, node)
		})

		AfterEach(func() {
			_ = os.Setenv(consts.PodNamespaceEnvKey, "")
			_ = os.Setenv(consts.NodeNameEnvKey, "")
		})

		It("should report error when gateway namespace is not found", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			gomock.InOrder(
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(nil, os.ErrNotExist),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(os.ErrNotExist))
		})

		It("should report error when failed to create wgCtrl client", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			gomock.InOrder(
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(nil, fmt.Errorf("failed")),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
		})

		It("should report error when failed to configure wireguard device", func() {
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6000", Index: 10}}
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
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().LinkList().Return([]netlink.Link{wg0}, nil),
				mnl.EXPECT().RouteList(wg0, netlink.FAMILY_V4).Return(nil, nil),
				mnl.EXPECT().RouteAdd(&netlink.Route{LinkIndex: 10, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(nil),
				mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(fmt.Errorf("failed")),
				mnl.EXPECT().RouteDel(&netlink.Route{LinkIndex: 10, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(nil),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
		})

		Context("test adding peer route", func() {
			It("should report error if failed to get wireguard link", func() {
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
				wg0 := &netlink.Wireguard{}
				gomock.InOrder(
					mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mnl.EXPECT().LinkByName("wg-6000").Return(wg0, fmt.Errorf("failed")),
					mclient.EXPECT().Close().Return(nil),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(errors.Unwrap(reconcileErr))).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error if failed to add route", func() {
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
				wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6000", Index: 10}}
				gomock.InOrder(
					mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
					mnl.EXPECT().LinkList().Return([]netlink.Link{wg0}, nil),
					mnl.EXPECT().RouteList(wg0, netlink.FAMILY_V4).Return(nil, nil),
					mnl.EXPECT().RouteAdd(&netlink.Route{LinkIndex: 10, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(fmt.Errorf("failed")),
					mclient.EXPECT().Close().Return(nil),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(errors.Unwrap(reconcileErr))).To(Equal(fmt.Errorf("failed")))
			})

			It("should succeed and update gateway status", func() {
				mns := r.NetNS.(*mocknetnswrapper.MockInterface)
				mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
				mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
				gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
				wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: "wg-6000", Index: 10}}
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
					mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
					mwg.EXPECT().New().Return(mclient, nil),
					mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
					mnl.EXPECT().LinkList().Return([]netlink.Link{wg0}, nil),
					mnl.EXPECT().RouteList(wg0, netlink.FAMILY_V4).Return(nil, nil),
					mnl.EXPECT().RouteAdd(&netlink.Route{LinkIndex: 10, Scope: netlink.SCOPE_LINK, Dst: getIPNet(podIPAddrNet)}).Return(nil),
					mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(nil),
					mclient.EXPECT().Close().Return(nil),
				)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				gwStatus := &egressgatewayv1alpha1.GatewayStatus{}
				err := getGatewayStatus(r.Client, gwStatus)
				Expect(err).To(BeNil())
				Expect(gwStatus.Spec.ReadyPeerConfigurations).To(Equal([]egressgatewayv1alpha1.PeerConfiguration{
					{
						PublicKey:     pubK,
						InterfaceName: "wg-6000",
						PodEndpoint:   fmt.Sprintf("%s/%s", testNamespace, testName),
					},
				}))
			})
		})
	})

	Context("Test updating gateway node status", func() {
		peerConfigs := []egressgatewayv1alpha1.PeerConfiguration{
			{
				PublicKey:     "pubk1",
				InterfaceName: "wg-6000",
			},
			{
				PublicKey:     "pubk2",
				InterfaceName: "wg-6001",
			},
		}

		BeforeEach(func() {
			_ = os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			_ = os.Setenv(consts.NodeNameEnvKey, testNodeName)
		})

		AfterEach(func() {
			_ = os.Setenv(consts.PodNamespaceEnvKey, "")
			_ = os.Setenv(consts.NodeNameEnvKey, "")
		})

		It("should create new gateway status object if not exist", func() {
			getTestReconciler(node)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, PeerUpdateOpAdd)
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
							PublicKey:     "pubk1",
							InterfaceName: "wg-6000",
						},
						{
							PublicKey:     "pubk3",
							InterfaceName: "wg-6002",
						},
					},
				},
			}
			getTestReconciler(node, existing)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, PeerUpdateOpAdd)
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
							PublicKey:     "pubk1",
							InterfaceName: "wg-6000",
						},
						{
							PublicKey:     "pubk3",
							InterfaceName: "ns3",
						},
					},
				},
			}
			peerConfigs = []egressgatewayv1alpha1.PeerConfiguration{
				{
					PublicKey:     "pubk3",
					InterfaceName: "wg-6000",
				},
			}
			getTestReconciler(node, existing)
			err := r.updateGatewayNodeStatus(context.TODO(), peerConfigs, PeerUpdateOpDelete)
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

			_ = os.Setenv(consts.PodNamespaceEnvKey, testPodNamespace)
			_ = os.Setenv(consts.NodeNameEnvKey, testNodeName)
		})

		AfterEach(func() {
			_ = os.Setenv(consts.PodNamespaceEnvKey, "")
			_ = os.Setenv(consts.NodeNameEnvKey, "")
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
							PublicKey:     pubK,
							InterfaceName: "wg-6000",
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
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
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
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().RouteList(wg0, netlink.FAMILY_ALL).Return([]netlink.Route{{Dst: getIPNet("10.0.0.1/32")}}, nil),
				mnl.EXPECT().RouteDel(&netlink.Route{Dst: getIPNet("10.0.0.1/32")}).Return(nil),
				mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(nil),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
			err := getGatewayStatus(r.Client, gwStatus)
			Expect(err).To(BeNil())
			Expect(gwStatus.Spec.ReadyPeerConfigurations).To(BeEmpty())
		})

		It("should clean peer and route when the PodEndpoint binding is invalid", func() {
			podEndpoint = getTestPodEndpoint()
			podEndpoint.Generation++
			pod = getTestPod()
			gwStatus := &egressgatewayv1alpha1.GatewayStatus{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testNodeName,
					Namespace: testPodNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayStatusSpec{
					ReadyPeerConfigurations: []egressgatewayv1alpha1.PeerConfiguration{
						{
							PodEndpoint:   fmt.Sprintf("%s/%s", testNamespace, testName),
							PublicKey:     pubK,
							InterfaceName: "wg-6000",
						},
					},
				},
			}
			gwConfig = getTestGwConfig()
			getTestReconciler(podEndpoint, pod, gwConfig, gwStatus)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			wg0 := &netlink.Wireguard{}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			pk, _ := wgtypes.ParseKey(pubK)
			device := &wgtypes.Device{
				Peers: []wgtypes.Peer{
					{
						PublicKey: pk,
						AllowedIPs: []net.IPNet{
							*getIPNet(podIPAddrNet),
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
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil),
				mnl.EXPECT().RouteList(wg0, netlink.FAMILY_ALL).Return([]netlink.Route{{Dst: getIPNet(podIPAddrNet)}}, nil),
				mnl.EXPECT().RouteDel(&netlink.Route{Dst: getIPNet(podIPAddrNet)}).Return(nil),
				mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(nil),
				mclient.EXPECT().Close().Return(nil),
			)

			_, reconcileErr = r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: testName, Namespace: testNamespace},
			})
			Expect(reconcileErr).To(BeNil())
			Expect(getGatewayStatus(r.Client, gwStatus)).To(Succeed())
			Expect(gwStatus.Spec.ReadyPeerConfigurations).To(BeEmpty())
		})

		It("should not clean existing peer and route", func() {
			podEndpoint = getTestPodEndpoint()
			podEndpoint.Name = testName + "a"
			podEndpoint.OwnerReferences[0].Name = podEndpoint.Name
			pod = getTestPod()
			pod.Name = podEndpoint.Name
			gwConfig = getTestGwConfig()
			getTestReconciler(podEndpoint, pod, gwConfig)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
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
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mclient.EXPECT().Close().Return(nil),
			)
			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})

		It("should preserve a route still owned by a retained peer", func() {
			podEndpoint = getTestPodEndpoint()
			podEndpoint.Name = testName + "a"
			podEndpoint.OwnerReferences[0].Name = podEndpoint.Name
			pod = getTestPod()
			pod.Name = podEndpoint.Name
			gwConfig = getTestGwConfig()
			getTestReconciler(podEndpoint, pod, gwConfig)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
			pk, _ := wgtypes.ParseKey(pubK)
			pk2, _ := wgtypes.ParseKey(pubK2)
			device := &wgtypes.Device{
				Peers: []wgtypes.Peer{
					{
						PublicKey: pk,
						AllowedIPs: []net.IPNet{
							*getIPNet(podIPAddrNet),
						},
					},
					{
						PublicKey: pk2,
						AllowedIPs: []net.IPNet{
							*getIPNet(podIPAddrNet),
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
			gomock.InOrder(
				mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil),
				mwg.EXPECT().New().Return(mclient, nil),
				mclient.EXPECT().Device("wg-6000").Return(device, nil),
				mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(nil),
				mclient.EXPECT().Close().Return(nil),
			)

			_, reconcileErr = r.Reconcile(context.TODO(), req)
			Expect(reconcileErr).To(BeNil())
		})

		It("should handle multiple gateway namespaces properly", func() {
			podEndpoint := getTestPodEndpoint()
			podEndpoint.Name = testName + "a"
			podEndpoint.OwnerReferences[0].Name = podEndpoint.Name
			pod := getTestPod()
			pod.Name = podEndpoint.Name
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
				podEndpoint,
				pod,
			}
			getTestReconciler(objects...)
			mns := r.NetNS.(*mocknetnswrapper.MockInterface)
			mwg := r.WgCtrl.(*mockwgctrlwrapper.MockInterface)
			mnl := r.Netlink.(*mocknetlinkwrapper.MockInterface)
			wg0 := &netlink.Wireguard{}
			gwns := &mocknetnswrapper.MockNetNS{Name: consts.GatewayNetnsName}
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
			// 1st gateway namespace, delete one peer and keep one peer
			mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(gwns, nil)
			mwg.EXPECT().New().Return(mclient, nil)
			mclient.EXPECT().Device("wg-6000").Return(device, nil)
			mnl.EXPECT().LinkByName("wg-6000").Return(wg0, nil)
			mnl.EXPECT().RouteList(wg0, netlink.FAMILY_ALL).Return([]netlink.Route{{Dst: getIPNet("10.0.0.1/32")}, {Dst: getIPNet("10.0.0.2/32")}}, nil)
			mnl.EXPECT().RouteDel(&netlink.Route{Dst: getIPNet("10.0.0.2/32")}).Return(nil)
			mclient.EXPECT().ConfigureDevice("wg-6000", config).Return(nil)
			mclient.EXPECT().Close().Return(nil)
			// 2nd gateway namespace, return error, should not block
			mns.EXPECT().GetNS(consts.GatewayNetnsName).Return(nil, fmt.Errorf("failed"))
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
