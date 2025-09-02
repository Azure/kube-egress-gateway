// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/interfaceclient/mock_interfaceclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient/mock_loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient/mock_publicipprefixclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient/mock_subnetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient/mock_virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetvmclient/mock_virtualmachinescalesetvmclient"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/compat"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

const (
	testRG              = "testRG"
	testLBName          = "testLB"
	testLBRG            = "testLBRG"
	testLBConfigUID     = "testLBConfig"
	testVnetName        = "testVnet"
	testVnetRG          = "testVnetRG"
	testSubnetName      = "testSubnet"
	testVMSSUID         = "testvmss"
	testGWConfigUID     = "testGWConfig"
	lbProbePort     int = 8082
)

var _ = Describe("GatewayLBConfiguration controller unit tests", func() {
	var (
		r        *GatewayLBConfigurationReconciler
		az       *azmanager.AzureManager
		recorder = record.NewFakeRecorder(10)
	)

	Context("Reconcile", func() {
		var (
			req           reconcile.Request
			res           reconcile.Result
			cl            client.Client
			reconcileErr  error
			getErr        error
			gwConfig      *egressgatewayv1alpha1.StaticGatewayConfiguration
			lbConfig      *egressgatewayv1alpha1.GatewayLBConfiguration
			foundLBConfig = &egressgatewayv1alpha1.GatewayLBConfiguration{}
			foundVMConfig = &egressgatewayv1alpha1.GatewayVMConfiguration{}
		)

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
				},
			}
			lbConfig = &egressgatewayv1alpha1.GatewayLBConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
					UID:       testLBConfigUID,
					OwnerReferences: []metav1.OwnerReference{
						{
							Name: testName,
							UID:  testGWConfigUID,
						},
					},
				},
				Spec: egressgatewayv1alpha1.GatewayLBConfigurationSpec{
					GatewayNodepoolName: "testgw",
					GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
						VmssResourceGroup:  "vmssRG",
						VmssName:           "vmss",
						PublicIpPrefixSize: 31,
					},
					ProvisionPublicIps: true,
				},
			}
		})

		When("lbConfig is not found", func() {
			It("should only report error in get", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				compatClient := compat.NewCompatClient(cl)
				r = &GatewayLBConfigurationReconciler{Client: cl, CompatClient: compatClient, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundLBConfig)

				Expect(reconcileErr).To(BeNil())
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		Context("TestGetGatewayVMSS", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				r = &GatewayLBConfigurationReconciler{AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
			})

			It("should return error when listing vmss fails", func() {
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed to list vmss"))
				vmss, err := r.getGatewayVMSS(context.Background(), lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("failed to list vmss")))
			})

			It("should return error when vmss in list does not have expected tag", func() {
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{
					{ID: to.Ptr("test")},
				}, nil)
				vmss, err := r.getGatewayVMSS(context.Background(), lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("gateway VMSS not found")))
			})

			It("should return expected vmss in list", func() {
				vmss := &compute.VirtualMachineScaleSet{Tags: map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")}}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{
					{ID: to.Ptr("dummy")},
					vmss,
				}, nil)
				foundVMSS, err := r.getGatewayVMSS(context.Background(), lbConfig)
				Expect(err).To(BeNil())
				Expect(to.Val(foundVMSS)).To(Equal(to.Val(vmss)))
			})

			It("should return error when getting vmss fails", func() {
				lbConfig.Spec.GatewayNodepoolName = ""
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().Get(gomock.Any(), "vmssRG", "vmss", gomock.Any()).Return(nil, fmt.Errorf("vmss not found"))
				vmss, err := r.getGatewayVMSS(context.Background(), lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("vmss not found")))
			})

			It("should return expected vmss from get", func() {
				lbConfig.Spec.GatewayNodepoolName = ""
				vmss := &compute.VirtualMachineScaleSet{ID: to.Ptr("test")}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().Get(gomock.Any(), "vmssRG", "vmss", gomock.Any()).Return(vmss, nil)
				foundVMSS, err := r.getGatewayVMSS(context.Background(), lbConfig)
				Expect(err).To(BeNil())
				Expect(to.Val(foundVMSS)).To(Equal(to.Val(vmss)))
			})
		})

		When("lbConfig has GatewayNodepoolName", func() {
			BeforeEach(func() {
				controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig).Build()
				compatClient := compat.NewCompatClient(cl)
				r = &GatewayLBConfigurationReconciler{Client: cl, CompatClient: compatClient, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
			})

			It("should report error if gateway LB is not found", func() {
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(nil, fmt.Errorf("lb not found"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("lb not found")))
				assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError lb not found"}, recorder.Events)
			})

			It("should report error if gateway VMSS is not found", func() {
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(&network.LoadBalancer{}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed to list vmss"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("failed to list vmss")))
				assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError failed to list vmss"}, recorder.Events)
			})

			It("should report error if gateway VMSS does not have UID", func() {
				vmss := &compute.VirtualMachineScaleSet{Tags: map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")}}
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(&network.LoadBalancer{}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("gateway vmss does not have UID")))
				assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError gateway vmss does not have UID"}, recorder.Events)
			})

			It("should report error if lb property is empty", func() {
				vmss := &compute.VirtualMachineScaleSet{
					Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
					Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
				}
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(&network.LoadBalancer{}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("lb property is empty")))
				assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError lb property is empty"}, recorder.Events)
			})

			Context("test frontend search", func() {
				frontendNotFoundErr := fmt.Errorf("found frontend(testvmss) with unexpected configuration")
				lb := &network.LoadBalancer{
					Properties: &network.LoadBalancerPropertiesFormat{
						FrontendIPConfigurations: []*network.FrontendIPConfiguration{},
					},
				}
				BeforeEach(func() {
					vmss := &compute.VirtualMachineScaleSet{
						Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
						Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
					}
					lb.Properties.FrontendIPConfigurations = append(lb.Properties.FrontendIPConfigurations, &network.FrontendIPConfiguration{
						Name: to.Ptr(testVMSSUID),
						ID:   r.GetLBFrontendIPConfigurationID(testVMSSUID),
					})
					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
					mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
					mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				})

				It("should report error if lb frontend does not properties", func() {
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
					assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError " + frontendNotFoundErr.Error()}, recorder.Events)
				})

				It("should report error if lb frontend does not have ip version", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
					assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError " + frontendNotFoundErr.Error()}, recorder.Events)
				})

				It("should report error if lb frontend does not have ipv4", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv6),
					}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
					assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError " + frontendNotFoundErr.Error()}, recorder.Events)
				})

				It("should report error if lb frontend does not have private ip", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
					}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
					assertEqualEvents([]string{"Warning ReconcileGatewayLBConfigurationError " + frontendNotFoundErr.Error()}, recorder.Events)
				})
			})

			It("should create new lb when it does not exist", func() {
				vmss := &compute.VirtualMachineScaleSet{
					Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
					Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
				}
				requestedLB, expectedLB := getExpectedLB(), getExpectedLB()
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(nil, &azcore.ResponseError{
					StatusCode: http.StatusNotFound,
				})
				mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(
					func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
						requestedLB.Properties.FrontendIPConfigurations[0].Properties.PrivateIPAddress = nil
						requestedLB.Properties.FrontendIPConfigurations[0].ID = nil
						requestedLB.Properties.BackendAddressPools[0].ID = nil
						Expect(equality.Semantic.DeepEqual(loadBalancer, *requestedLB)).To(BeTrue())
						return expectedLB, nil
					})
				mockSubnetClient := az.SubnetClient.(*mock_subnetclient.MockInterface)
				mockSubnetClient.EXPECT().Get(gomock.Any(), testVnetRG, testVnetName, testSubnetName, gomock.Any()).Return(&network.Subnet{
					ID: to.Ptr("testSubnet"),
				}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)

				Expect(getResource(cl, foundLBConfig)).ShouldNot(HaveOccurred())
				Expect(controllerutil.ContainsFinalizer(foundLBConfig, consts.LBConfigFinalizerName)).To(BeTrue())
			})

			Context("reconcile lbRule, lbProbe and vmConfig", func() {
				BeforeEach(func() {
					vmss := &compute.VirtualMachineScaleSet{
						Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
						Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
					}
					mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
					mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil).AnyTimes()
				})

				It("should create new lbRule and lbProbe", func() {
					lb := getEmptyLB()
					expectedLB := getExpectedLB()
					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
					mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
						Expect(equality.Semantic.DeepEqual(loadBalancer, *expectedLB)).To(BeTrue())
						return expectedLB, nil
					})
					res, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(BeNil())
					Expect(res).To(Equal(ctrl.Result{}))

					getErr = getResource(cl, foundLBConfig)
					Expect(getErr).To(BeNil())
					Expect(foundLBConfig.Status.FrontendIp).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
					assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
				})

				It("should not update LB when lb rule and probe are expected", func() {
					expectedLB := getExpectedLB()
					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(expectedLB, nil)
					res, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(BeNil())
					Expect(res).To(Equal(ctrl.Result{}))
					Expect(foundLBConfig.Status.FrontendIp).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
					assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
				})

				It("should drop incorrect lbRule and create new one", func() {
					existingLB, expectedLB := getExpectedLB(), getExpectedLB()
					existingLB.Properties.LoadBalancingRules[0].Properties.Protocol = to.Ptr(network.TransportProtocolTCP)
					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(existingLB, nil)
					mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
						Expect(equality.Semantic.DeepEqual(loadBalancer, *expectedLB)).To(BeTrue())
						return expectedLB, nil
					})
					res, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(BeNil())
					Expect(res).To(Equal(ctrl.Result{}))

					getErr = getResource(cl, foundLBConfig)
					Expect(getErr).To(BeNil())
					Expect(foundLBConfig.Status.FrontendIp).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
					assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
				})

				It("should drop incorrect lbProbe and create new one", func() {
					existingLB, expectedLB := getExpectedLB(), getExpectedLB()
					for _, prop := range []*network.ProbePropertiesFormat{
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName + "1"),
							Protocol:    to.Ptr(network.ProbeProtocolHTTP),
							Port:        to.Ptr(int32(lbProbePort)),
						},
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName),
							Protocol:    to.Ptr(network.ProbeProtocolTCP),
							Port:        to.Ptr(int32(lbProbePort)),
						},
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName),
							Protocol:    to.Ptr(network.ProbeProtocolHTTP),
							Port:        to.Ptr(int32(lbProbePort + 1)),
						},
					} {
						existingLB.Properties.Probes[0].Properties = prop
						mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
						mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(existingLB, nil)
						mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
							Expect(equality.Semantic.DeepEqual(loadBalancer, *expectedLB)).To(BeTrue())
							return expectedLB, nil
						})
						res, reconcileErr = r.Reconcile(context.TODO(), req)
						Expect(reconcileErr).To(BeNil())
						Expect(res).To(Equal(ctrl.Result{}))

						getErr = getResource(cl, foundLBConfig)
						Expect(getErr).To(BeNil())
						Expect(foundLBConfig.Status.FrontendIp).To(Equal("10.0.0.4"))
						Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
						assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
					}
				})

				It("should select an unoccupied port", func() {
					existingLB, expectedLB := getExpectedLB(), getExpectedLB()
					existingLB.Properties.LoadBalancingRules[0].Name = to.Ptr(testLBConfigUID + "1")
					existingLB.Properties.Probes[0].Name = to.Ptr(testLBConfigUID + "1")
					expectedLB.Properties.LoadBalancingRules = append(existingLB.Properties.LoadBalancingRules, expectedLB.Properties.LoadBalancingRules...)
					expectedLB.Properties.Probes = append(existingLB.Properties.Probes, expectedLB.Properties.Probes...)
					expectedLB.Properties.LoadBalancingRules[1].Properties.FrontendPort = to.Ptr(int32(6001))
					expectedLB.Properties.LoadBalancingRules[1].Properties.BackendPort = to.Ptr(int32(6001))

					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(existingLB, nil)
					mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
						Expect(equality.Semantic.DeepEqual(loadBalancer, *expectedLB)).To(BeTrue())
						return expectedLB, nil
					})
					res, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(BeNil())
					Expect(res).To(Equal(ctrl.Result{}))

					getErr = getResource(cl, foundLBConfig)
					Expect(getErr).To(BeNil())
					Expect(foundLBConfig.Status.FrontendIp).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6001)))
					assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
				})
			})
		})

		When("lb is reconciled", func() {
			BeforeEach(func() {
				controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				expectedLB := getExpectedLB()
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(expectedLB, nil)
				vmss := &compute.VirtualMachineScaleSet{
					Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
					Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
				}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil).AnyTimes()
			})

			It("should create a new vmConfig", func() {
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig).Build()
				compatClient := compat.NewCompatClient(cl)
				r = &GatewayLBConfigurationReconciler{Client: cl, CompatClient: compatClient, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err := getResource(cl, foundVMConfig)
				Expect(err).To(BeNil())

				Expect(foundVMConfig.Spec.GatewayNodepoolName).To(Equal(lbConfig.Spec.GatewayNodepoolName))
				Expect(foundVMConfig.Spec.GatewayVmssProfile).To(Equal(lbConfig.Spec.GatewayVmssProfile))
				Expect(foundVMConfig.Spec.PublicIpPrefixId).To(Equal(lbConfig.Spec.PublicIpPrefixId))
				Expect(foundVMConfig.Spec.ProvisionPublicIps).To(Equal(lbConfig.Spec.ProvisionPublicIps))

				existing := metav1.GetControllerOf(foundVMConfig)
				Expect(existing).NotTo(BeNil())
				Expect(existing.Name).To(Equal(testName))

				getErr = getResource(cl, foundLBConfig)
				Expect(getErr).To(BeNil())
				Expect(foundLBConfig.Status.EgressIpPrefix).To(BeEmpty())
				assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
			})

			It("should update status from existing vmConfig", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
						GatewayNodepoolName: "testgw",
						GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
							VmssResourceGroup:  "vmssRG",
							VmssName:           "vmss",
							PublicIpPrefixSize: 31,
						},
						PublicIpPrefixId:   "testPipPrefix",
						ProvisionPublicIps: true,
					},
					Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
						EgressIpPrefix: "1.2.3.4/31",
					},
				}

				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				getErr = getResource(cl, foundLBConfig)
				Expect(getErr).To(BeNil())
				Expect(foundLBConfig.Status.EgressIpPrefix).To(Equal("1.2.3.4/31"))
				assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
			})

			It("should update existing vmConfig accordingly", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
						GatewayNodepoolName: "testgw1",
						GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
							VmssResourceGroup:  "vmssRG1",
							VmssName:           "vmss1",
							PublicIpPrefixSize: 30,
						},
						PublicIpPrefixId: "testPipPrefix1",
					},
					Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
						EgressIpPrefix: "1.2.3.4/31",
					},
				}

				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				getErr = getResource(cl, foundVMConfig)
				Expect(getErr).To(BeNil())
				Expect(foundVMConfig.Spec.GatewayNodepoolName).To(Equal(lbConfig.Spec.GatewayNodepoolName))
				Expect(foundVMConfig.Spec.ProvisionPublicIps).To(Equal(lbConfig.Spec.ProvisionPublicIps))
				assertEqualEvents([]string{"Normal ReconcileGatewayLBConfigurationSuccess GatewayLBConfiguration reconciled"}, recorder.Events)
			})
		})

		When("deleting a lbConfig with finalizer and vmConfig", func() {
			BeforeEach(func() {
				controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
				lbConfig.ObjectMeta.DeletionTimestamp = to.Ptr(metav1.Now())
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
			})

			It("should delete vmConfig before cleaning lb", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
				}
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundVMConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})
		})

		When("deleting a lbConfig with finalizer but no vmConfig", func() {
			BeforeEach(func() {
				controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
				lbConfig.ObjectMeta.DeletionTimestamp = to.Ptr(metav1.Now())
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(lbConfig).WithRuntimeObjects(gwConfig, lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az, Recorder: recorder, LBProbePort: lbProbePort}
				vmss := &compute.VirtualMachineScaleSet{
					Properties: &compute.VirtualMachineScaleSetProperties{UniqueID: to.Ptr(testVMSSUID)},
					Tags:       map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")},
				}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
			})

			It("should delete lb and lbConfig if lb does not need cleanup", func() {
				lb := getEmptyLB()
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
				mockLoadBalancerClient.EXPECT().Delete(gomock.Any(), testLBRG, testLBName).Return(nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundLBConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})

			It("should delete lb and delete lbConfig", func() {
				lb := getExpectedLB()
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
				mockLoadBalancerClient.EXPECT().Delete(gomock.Any(), testLBRG, testLBName).Return(nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundLBConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})

			It("should return error and not delete lbConfig if reconcile() returns error", func() {
				lb := getExpectedLB()
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
				mockLoadBalancerClient.EXPECT().Delete(gomock.Any(), testLBRG, testLBName).Return(fmt.Errorf("failed to delete lb"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("failed to delete lb")))
				getErr = getResource(cl, foundLBConfig)
				Expect(getErr).To(BeNil())
				Expect(controllerutil.ContainsFinalizer(foundLBConfig, consts.LBConfigFinalizerName)).To(BeTrue())
			})

			It("should not delete lb but just the rules when there are other rules referencing the same frontend/backend", func() {
				lb := getExpectedLB()
				additionalRule := &network.LoadBalancingRule{
					Name: to.Ptr("additional"),
					Properties: &network.LoadBalancingRulePropertiesFormat{
						FrontendIPConfiguration: &network.SubResource{
							ID: to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s", testLBRG, testLBName, testVMSSUID)),
						},
						BackendAddressPool: &network.SubResource{
							ID: to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s", testLBRG, testLBName, testVMSSUID)),
						},
					},
				}
				lb.Properties.LoadBalancingRules = append(lb.Properties.LoadBalancingRules, additionalRule)
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(lb, nil)
				mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), testLBRG, testLBName, gomock.Any()).DoAndReturn(
					func(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
						emptyLB := getEmptyLB()
						emptyLB.Properties.LoadBalancingRules = append(emptyLB.Properties.LoadBalancingRules, additionalRule)
						Expect(equality.Semantic.DeepEqual(loadBalancer, *emptyLB)).To(BeTrue())
						return emptyLB, nil
					})
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundLBConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})
		})

		Context("TestSameLBRuleConfig", func() {
			tests := []struct {
				rule1   *network.LoadBalancingRule
				rule2   *network.LoadBalancingRule
				areSame bool
			}{
				{
					rule1:   &network.LoadBalancingRule{},
					rule2:   &network.LoadBalancingRule{},
					areSame: true,
				},
				{
					rule1:   &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{}},
					rule2:   &network.LoadBalancingRule{},
					areSame: false,
				},
				{
					rule1: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						FrontendIPConfiguration: &network.SubResource{ID: to.Ptr("123")},
					}},
					rule2:   &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{}},
					areSame: false,
				},
				{
					rule1: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						FrontendIPConfiguration: &network.SubResource{ID: to.Ptr("123")},
						BackendAddressPool:      &network.SubResource{ID: to.Ptr("123")},
					}},
					rule2: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						FrontendIPConfiguration: &network.SubResource{ID: to.Ptr("123")},
					}},
					areSame: false,
				},
				{
					rule1: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						Probe: &network.SubResource{ID: to.Ptr("123")},
					}},
					rule2: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						Probe: &network.SubResource{ID: to.Ptr("456")},
					}},
					areSame: false,
				},
				{
					rule1: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						Protocol: to.Ptr(network.TransportProtocolTCP),
					}},
					rule2: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						Protocol: to.Ptr(network.TransportProtocolUDP),
					}},
					areSame: false,
				},
				{
					rule1: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						EnableFloatingIP: to.Ptr(true),
					}},
					rule2: &network.LoadBalancingRule{Properties: &network.LoadBalancingRulePropertiesFormat{
						EnableFloatingIP: to.Ptr(false),
					}},
					areSame: false,
				},
			}
			for _, c := range tests {
				same := sameLBRuleConfig(context.Background(), c.rule1, c.rule2)
				Expect(same).To(Equal(c.areSame))
			}
		})

		Context("TestSelectPortForLBRule", func() {
			targetRule := &network.LoadBalancingRule{
				Properties: &network.LoadBalancingRulePropertiesFormat{
					BackendAddressPool: &network.SubResource{ID: to.Ptr("123")},
				},
			}
			var lbRules []*network.LoadBalancingRule
			invalidErr := fmt.Errorf("selectPortForLBRule: found rule with invalid LB port")

			BeforeEach(func() {
				lbRules = make([]*network.LoadBalancingRule, 0)
			})
			It("should report error is rule does not have frontend port", func() {
				lbRules = append(lbRules, &network.LoadBalancingRule{
					Properties: &network.LoadBalancingRulePropertiesFormat{
						BackendAddressPool: &network.SubResource{ID: to.Ptr("123")},
					},
				})
				_, err := selectPortForLBRule(targetRule, lbRules)
				Expect(err).To(Equal(invalidErr))
			})

			It("should report error is rule port does not in valid range", func() {
				lbRules = append(lbRules, &network.LoadBalancingRule{
					Properties: &network.LoadBalancingRulePropertiesFormat{
						BackendAddressPool: &network.SubResource{ID: to.Ptr("123")},
						FrontendPort:       to.Ptr(int32(100)),
						BackendPort:        to.Ptr(int32(100)),
					},
				})
				_, err := selectPortForLBRule(targetRule, lbRules)
				Expect(err).To(Equal(invalidErr))
			})

			It("should report error when all ports are occupied", func() {
				for i := consts.WireguardPortStart; i < consts.WireguardPortEnd; i++ {
					lbRules = append(lbRules, &network.LoadBalancingRule{
						Properties: &network.LoadBalancingRulePropertiesFormat{
							BackendAddressPool: &network.SubResource{ID: to.Ptr("123")},
							FrontendPort:       to.Ptr(int32(i)),
							BackendPort:        to.Ptr(int32(i)),
						},
					})
				}
				_, err := selectPortForLBRule(targetRule, lbRules)
				Expect(err).To(Equal(fmt.Errorf("selectPortForLBRule: No available ports")))
			})
		})
	})
})

func getMockAzureManager(ctrl *gomock.Controller) *azmanager.AzureManager {
	conf := &config.CloudConfig{
		ARMClientConfig: azclient.ARMClientConfig{
			Cloud:     "AzureTest",
			UserAgent: "testUserAgent",
		},
		Location:                  "location",
		SubscriptionID:            "testSub",
		ResourceGroup:             testRG,
		LoadBalancerName:          testLBName,
		LoadBalancerResourceGroup: testLBRG,
		VnetName:                  testVnetName,
		VnetResourceGroup:         testVnetRG,
		SubnetName:                testSubnetName,
	}
	factory := mock_azclient.NewMockClientFactory(ctrl)
	factory.EXPECT().GetLoadBalancerClient().Return(mock_loadbalancerclient.NewMockInterface(ctrl))
	factory.EXPECT().GetVirtualMachineScaleSetClient().Return(mock_virtualmachinescalesetclient.NewMockInterface(ctrl))
	factory.EXPECT().GetVirtualMachineScaleSetVMClient().Return(mock_virtualmachinescalesetvmclient.NewMockInterface(ctrl))
	factory.EXPECT().GetPublicIPPrefixClient().Return(mock_publicipprefixclient.NewMockInterface(ctrl))
	factory.EXPECT().GetInterfaceClient().Return(mock_interfaceclient.NewMockInterface(ctrl))
	factory.EXPECT().GetSubnetClient().Return(mock_subnetclient.NewMockInterface(ctrl))
	az, _ := azmanager.CreateAzureManager(conf, factory)
	return az
}

func getEmptyLB() *network.LoadBalancer {
	return &network.LoadBalancer{
		Name:     to.Ptr(testLBName),
		Location: to.Ptr("location"),
		SKU: &network.LoadBalancerSKU{
			Name: to.Ptr(network.LoadBalancerSKUNameStandard),
			Tier: to.Ptr(network.LoadBalancerSKUTierRegional),
		},
		Properties: &network.LoadBalancerPropertiesFormat{
			FrontendIPConfigurations: []*network.FrontendIPConfiguration{
				{
					Name: to.Ptr(testVMSSUID),
					ID:   to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s", testLBRG, testLBName, testVMSSUID)),
					Properties: &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion:   to.Ptr(network.IPVersionIPv4),
						PrivateIPAllocationMethod: to.Ptr(network.IPAllocationMethodDynamic),
						PrivateIPAddress:          to.Ptr("10.0.0.4"),
						Subnet:                    &network.Subnet{ID: to.Ptr("testSubnet")},
					},
				},
			},
			BackendAddressPools: []*network.BackendAddressPool{
				{
					Name:       to.Ptr(testVMSSUID),
					ID:         to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s", testLBRG, testLBName, testVMSSUID)),
					Properties: &network.BackendAddressPoolPropertiesFormat{},
				},
			},
		},
	}
}

func getExpectedLB() *network.LoadBalancer {
	return &network.LoadBalancer{
		Name:     to.Ptr(testLBName),
		Location: to.Ptr("location"),
		SKU: &network.LoadBalancerSKU{
			Name: to.Ptr(network.LoadBalancerSKUNameStandard),
			Tier: to.Ptr(network.LoadBalancerSKUTierRegional),
		},
		Properties: &network.LoadBalancerPropertiesFormat{
			FrontendIPConfigurations: []*network.FrontendIPConfiguration{
				{
					Name: to.Ptr(testVMSSUID),
					ID:   to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s", testLBRG, testLBName, testVMSSUID)),
					Properties: &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion:   to.Ptr(network.IPVersionIPv4),
						PrivateIPAllocationMethod: to.Ptr(network.IPAllocationMethodDynamic),
						PrivateIPAddress:          to.Ptr("10.0.0.4"),
						Subnet:                    &network.Subnet{ID: to.Ptr("testSubnet")},
					},
				},
			},
			BackendAddressPools: []*network.BackendAddressPool{
				{
					Name:       to.Ptr(testVMSSUID),
					ID:         to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s", testLBRG, testLBName, testVMSSUID)),
					Properties: &network.BackendAddressPoolPropertiesFormat{},
				},
			},
			LoadBalancingRules: []*network.LoadBalancingRule{
				{
					Name: to.Ptr(testLBConfigUID),
					Properties: &network.LoadBalancingRulePropertiesFormat{
						Protocol:         to.Ptr(network.TransportProtocolUDP),
						EnableFloatingIP: to.Ptr(true),
						FrontendIPConfiguration: &network.SubResource{
							ID: to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s", testLBRG, testLBName, testVMSSUID)),
						},
						BackendAddressPool: &network.SubResource{
							ID: to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s", testLBRG, testLBName, testVMSSUID)),
						},
						Probe: &network.SubResource{
							ID: to.Ptr(fmt.Sprintf("/subscriptions/testSub/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s", testLBRG, testLBName, testLBConfigUID)),
						},
						FrontendPort: to.Ptr(int32(6000)),
						BackendPort:  to.Ptr(int32(6000)),
					},
				},
			},
			Probes: []*network.Probe{
				{
					Name: to.Ptr(testLBConfigUID),
					Properties: &network.ProbePropertiesFormat{
						RequestPath: to.Ptr("/gw/" + testGWConfigUID),
						Protocol:    to.Ptr(network.ProbeProtocolHTTP),
						Port:        to.Ptr(int32(lbProbePort)),
					},
				},
			},
		},
	}
}

// Helper function to create reconciler with compatibility client
func createReconciler(cl client.Client, az *azmanager.AzureManager) *GatewayLBConfigurationReconciler {
	compatClient := compat.NewCompatClient(cl)
	return &GatewayLBConfigurationReconciler{
		Client:       cl,
		CompatClient: compatClient,
		AzureManager: az,
		Recorder:     record.NewFakeRecorder(10),
		LBProbePort:  lbProbePort,
	}
}

func assertEqualEvents(expected []string, actual <-chan string) {
	for _, e := range expected {
		Eventually(actual).Should(Receive(Equal(e)))
	}
	Consistently(actual).ShouldNot(Receive())
}
