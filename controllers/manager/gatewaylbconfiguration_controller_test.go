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
package manager

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient/mock_loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient/mock_virtualmachinescalesetclient"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/subnetclient/mocksubnetclient"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

const (
	testRG          = "testRG"
	testLBName      = "testLB"
	testLBRG        = "testLBRG"
	testLBConfigUID = "testLBConfig"
	testVnetName    = "testVnet"
	testVnetRG      = "testVnetRG"
	testSubnetName  = "testSubnet"
	testVMSSUID     = "testvmss"
	testGWConfigUID = "testGWConfig"
)

var _ = Describe("GatewayLBConfiguration controller unit tests", func() {
	var (
		s  = scheme.Scheme
		r  *GatewayLBConfigurationReconciler
		az *azmanager.AzureManager
	)

	Context("Reconcile", func() {
		var (
			req           reconcile.Request
			res           reconcile.Result
			cl            client.Client
			reconcileErr  error
			getErr        error
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
					GatewayVMSSProfile: egressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  "vmssRG",
						VMSSName:           "vmss",
						PublicIpPrefixSize: 31,
					},
				},
			}
			s.AddKnownTypes(egressgatewayv1alpha1.GroupVersion, lbConfig,
				&egressgatewayv1alpha1.StaticGatewayConfiguration{},
				&egressgatewayv1alpha1.GatewayVMConfiguration{})
		})

		When("lbConfig is not found", func() {
			It("should only report error in get", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundLBConfig)

				Expect(reconcileErr).To(BeNil())
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("lbConfig is newly created", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundLBConfig)
			})

			It("should not error", func() {
				Expect(reconcileErr).To(BeNil())
				Expect(getErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})

			It("should add finalizer", func() {
				Expect(controllerutil.ContainsFinalizer(foundLBConfig, consts.LBConfigFinalizerName)).To(BeTrue())
			})
		})

		Context("TestGetGatewayVMSS", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				r = &GatewayLBConfigurationReconciler{AzureManager: az}
			})

			It("should return error when listing vmss fails", func() {
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed to list vmss"))
				vmss, err := r.getGatewayVMSS(lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("failed to list vmss")))
			})

			It("should return error when vmss in list does not have expected tag", func() {
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{
					&compute.VirtualMachineScaleSet{ID: to.Ptr("test")},
				}, nil)
				vmss, err := r.getGatewayVMSS(lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("gateway VMSS not found")))
			})

			It("should return expected vmss in list", func() {
				vmss := &compute.VirtualMachineScaleSet{Tags: map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")}}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{
					&compute.VirtualMachineScaleSet{ID: to.Ptr("dummy")},
					vmss,
				}, nil)
				foundVMSS, err := r.getGatewayVMSS(lbConfig)
				Expect(err).To(BeNil())
				Expect(to.Val(foundVMSS)).To(Equal(to.Val(vmss)))
			})

			It("should return error when getting vmss fails", func() {
				lbConfig.Spec.GatewayNodepoolName = ""
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().Get(gomock.Any(), "vmssRG", "vmss").Return(nil, fmt.Errorf("vmss not found"))
				vmss, err := r.getGatewayVMSS(lbConfig)
				Expect(vmss).To(BeNil())
				Expect(err).To(Equal(fmt.Errorf("vmss not found")))
			})

			It("should return expected vmss from get", func() {
				lbConfig.Spec.GatewayNodepoolName = ""
				vmss := &compute.VirtualMachineScaleSet{ID: to.Ptr("test")}
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().Get(gomock.Any(), "vmssRG", "vmss").Return(vmss, nil)
				foundVMSS, err := r.getGatewayVMSS(lbConfig)
				Expect(err).To(BeNil())
				Expect(to.Val(foundVMSS)).To(Equal(to.Val(vmss)))
			})
		})

		When("lbConfig has finalizer and GatewayNodepoolName", func() {
			BeforeEach(func() {
				controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
			})

			It("should report error if gateway LB is not found", func() {
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(nil, fmt.Errorf("lb not found"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("lb not found")))
			})

			It("should report error if gateway VMSS is not found", func() {
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(&network.LoadBalancer{}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed to list vmss"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("failed to list vmss")))
			})

			It("should report error if gateway VMSS does not have UID", func() {
				vmss := &compute.VirtualMachineScaleSet{Tags: map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")}}
				mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
				mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(&network.LoadBalancer{}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("gateway vmss does not have UID")))
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
				})

				It("should report error if lb frontend does not have ip version", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
				})

				It("should report error if lb frontend does not have ipv4", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv6),
					}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
				})

				It("should report error if lb frontend does not have private ip", func() {
					lb.Properties.FrontendIPConfigurations[0].Properties = &network.FrontendIPConfigurationPropertiesFormat{
						PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
					}
					_, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(Equal(frontendNotFoundErr))
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
				mockSubnetClient := az.SubnetClient.(*mocksubnetclient.MockInterface)
				mockSubnetClient.EXPECT().Get(gomock.Any(), testVnetRG, testVnetName, testSubnetName, gomock.Any()).Return(&network.Subnet{
					ID: to.Ptr("testSubnet"),
				}, nil)
				mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
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
					Expect(foundLBConfig.Status.FrontendIP).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
				})

				It("should not update LB when lb rule and probe are expected", func() {
					expectedLB := getExpectedLB()
					mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
					mockLoadBalancerClient.EXPECT().Get(gomock.Any(), testLBRG, testLBName, gomock.Any()).Return(expectedLB, nil)
					res, reconcileErr = r.Reconcile(context.TODO(), req)
					Expect(reconcileErr).To(BeNil())
					Expect(res).To(Equal(ctrl.Result{}))
					Expect(foundLBConfig.Status.FrontendIP).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
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
					Expect(foundLBConfig.Status.FrontendIP).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
				})

				It("should drop incorrect lbProbe and create new one", func() {
					existingLB, expectedLB := getExpectedLB(), getExpectedLB()
					for _, prop := range []*network.ProbePropertiesFormat{
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName + "1"),
							Protocol:    to.Ptr(network.ProbeProtocolHTTP),
							Port:        to.Ptr(consts.WireguardDaemonServicePort),
						},
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName),
							Protocol:    to.Ptr(network.ProbeProtocolTCP),
							Port:        to.Ptr(consts.WireguardDaemonServicePort),
						},
						{
							RequestPath: to.Ptr("/" + testNamespace + "/" + testName),
							Protocol:    to.Ptr(network.ProbeProtocolHTTP),
							Port:        to.Ptr(consts.WireguardDaemonServicePort + 1),
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
						Expect(foundLBConfig.Status.FrontendIP).To(Equal("10.0.0.4"))
						Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6000)))
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
					Expect(foundLBConfig.Status.FrontendIP).To(Equal("10.0.0.4"))
					Expect(foundLBConfig.Status.ServerPort).To(Equal(int32(6001)))
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
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				err := getResource(cl, foundVMConfig)
				Expect(err).To(BeNil())

				Expect(foundVMConfig.Spec.GatewayNodepoolName).To(Equal(lbConfig.Spec.GatewayNodepoolName))
				Expect(foundVMConfig.Spec.GatewayVMSSProfile).To(Equal(lbConfig.Spec.GatewayVMSSProfile))
				Expect(foundVMConfig.Spec.PublicIpPrefixId).To(Equal(lbConfig.Spec.PublicIpPrefixId))

				existing := metav1.GetControllerOf(foundVMConfig)
				Expect(existing).NotTo(BeNil())
				Expect(existing.Name).To(Equal(testName))

				getErr = getResource(cl, foundLBConfig)
				Expect(getErr).To(BeNil())
				Expect(foundLBConfig.Status.PublicIpPrefix).To(BeEmpty())
			})

			It("should update status from existing vmConfig", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
						GatewayNodepoolName: "testgw",
						GatewayVMSSProfile: egressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  "vmssRG",
							VMSSName:           "vmss",
							PublicIpPrefixSize: 31,
						},
						PublicIpPrefixId: "testPipPrefix",
					},
					Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
						EgressIpPrefix: "1.2.3.4/31",
					},
				}

				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				getErr = getResource(cl, foundLBConfig)
				Expect(getErr).To(BeNil())
				Expect(foundLBConfig.Status.PublicIpPrefix).To(Equal("1.2.3.4/31"))
			})

			It("should update existing vmConfig accordingly", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:      testName,
						Namespace: testNamespace,
					},
					Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
						GatewayNodepoolName: "testgw1",
						GatewayVMSSProfile: egressgatewayv1alpha1.GatewayVMSSProfile{
							VMSSResourceGroup:  "vmssRG1",
							VMSSName:           "vmss1",
							PublicIpPrefixSize: 30,
						},
						PublicIpPrefixId: "testPipPrefix1",
					},
					Status: &egressgatewayv1alpha1.GatewayVMConfigurationStatus{
						EgressIpPrefix: "1.2.3.4/31",
					},
				}

				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))

				getErr = getResource(cl, foundVMConfig)
				Expect(getErr).To(BeNil())
				Expect(foundVMConfig.Spec.GatewayNodepoolName).To(Equal(lbConfig.Spec.GatewayNodepoolName))
			})
		})

		When("deleting lbConfig without finalizer", func() {
			BeforeEach(func() {
				lbConfig.ObjectMeta.DeletionTimestamp = to.Ptr(metav1.Now())
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
			})

			It("should not do anything", func() {
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundLBConfig)
				Expect(res).To(Equal(ctrl.Result{}))
				Expect(reconcileErr).To(BeNil())
				Expect(getErr).To(BeNil())
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
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundVMConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})

			It("should wait for vmConfig to be deleted before cleaning lb", func() {
				vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
					ObjectMeta: metav1.ObjectMeta{
						Name:              testName,
						Namespace:         testNamespace,
						DeletionTimestamp: to.Ptr(metav1.Now()),
					},
				}
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig, vmConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
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
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(lbConfig).Build()
				r = &GatewayLBConfigurationReconciler{Client: cl, AzureManager: az}
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
		Cloud:                     "AzureTest",
		Location:                  "location",
		SubscriptionID:            "testSub",
		UserAgent:                 "testUserAgent",
		ResourceGroup:             testRG,
		LoadBalancerName:          testLBName,
		LoadBalancerResourceGroup: testLBRG,
		VnetName:                  testVnetName,
		VnetResourceGroup:         testVnetRG,
		SubnetName:                testSubnetName,
	}
	az, _ := azmanager.CreateAzureManager(conf, azureclients.NewMockAzureClientsFactory(ctrl))
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
				&network.FrontendIPConfiguration{
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
				&network.BackendAddressPool{
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
				&network.FrontendIPConfiguration{
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
				&network.BackendAddressPool{
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
						Port:        to.Ptr(consts.WireguardDaemonServicePort),
					},
				},
			},
		},
	}
}
