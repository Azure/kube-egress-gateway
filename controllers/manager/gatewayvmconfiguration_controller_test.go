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
	"errors"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient/mockpublicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient/mockvmssclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient/mockvmssvmclient"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	vmssName = "vmss"
	vmssRG   = "vmssRG"
)

var _ = Describe("GatewayVMConfiguration controller unit tests", func() {
	var (
		s  = scheme.Scheme
		r  *GatewayVMConfigurationReconciler
		az *azmanager.AzureManager
	)

	Context("Reconcile", func() {
		var (
			req           reconcile.Request
			res           reconcile.Result
			cl            client.Client
			reconcileErr  error
			getErr        error
			vmConfig      *egressgatewayv1alpha1.GatewayVMConfiguration
			foundVMConfig = &egressgatewayv1alpha1.GatewayVMConfiguration{}
		)

		BeforeEach(func() {
			req = reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      testName,
					Namespace: testNamespace,
				},
			}
			vmConfig = &egressgatewayv1alpha1.GatewayVMConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testName,
					Namespace: testNamespace,
				},
				Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
					GatewayNodepoolName: "testgw",
					GatewayVMSSProfile: egressgatewayv1alpha1.GatewayVMSSProfile{
						VMSSResourceGroup:  vmssRG,
						VMSSName:           vmssName,
						PublicIpPrefixSize: 31,
					},
				},
			}
			s.AddKnownTypes(egressgatewayv1alpha1.GroupVersion, vmConfig,
				&egressgatewayv1alpha1.StaticGatewayConfiguration{},
				&egressgatewayv1alpha1.GatewayLBConfiguration{})
		})

		When("vmConfig is not found", func() {
			It("should only report error in get", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
				r = &GatewayVMConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundVMConfig)

				Expect(reconcileErr).To(BeNil())
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
				Expect(res).To(Equal(ctrl.Result{}))
			})
		})

		When("vmConfig is newly created", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(vmConfig).Build()
				r = &GatewayVMConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				res, reconcileErr = r.Reconcile(context.TODO(), req)
				getErr = getResource(cl, foundVMConfig)
			})

			It("should not error", func() {
				Expect(reconcileErr).To(BeNil())
				Expect(getErr).To(BeNil())
				Expect(res).To(Equal(ctrl.Result{}))
			})

			It("should add finalizer", func() {
				Expect(controllerutil.ContainsFinalizer(foundVMConfig, consts.VMConfigFinalizerName)).To(BeTrue())
			})
		})

		Context("TestGetGatewayVMSS", func() {
			It("should return vmss or error as expected", func() {
				tests := []struct {
					desc        string
					vmss        *compute.VirtualMachineScaleSet
					expectGet   bool
					returnedErr error
					expectedErr error
				}{
					{
						desc:        "should return error when listing vmss fails",
						returnedErr: fmt.Errorf("failed to list vmss"),
						expectedErr: fmt.Errorf("failed to list vmss"),
					},
					{
						desc:        "should return error when vmss in list does not have nodepool name tag",
						vmss:        &compute.VirtualMachineScaleSet{ID: to.Ptr("test")},
						expectedErr: fmt.Errorf("gateway VMSS not found"),
					},
					{
						desc:        "should return error when vmss in list does not have ip prefix length tag",
						vmss:        &compute.VirtualMachineScaleSet{Tags: map[string]*string{consts.AKSNodepoolTagKey: to.Ptr("testgw")}},
						expectedErr: fmt.Errorf("nodepool does not have IP prefix size"),
					},
					{
						desc: "should return error when vmss in list has invalid ip prefix length tag",
						vmss: &compute.VirtualMachineScaleSet{
							Tags: map[string]*string{
								consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
								consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("0"),
							},
						},
						expectedErr: fmt.Errorf("failed to parse nodepool IP prefix size: 0"),
					},
					{
						desc: "should return correct vmss in list",
						vmss: &compute.VirtualMachineScaleSet{
							Tags: map[string]*string{
								consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
								consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
							},
						},
					},
					{
						desc:        "should return error when getting vmss fails",
						expectGet:   true,
						returnedErr: fmt.Errorf("failed to get vmss"),
						expectedErr: fmt.Errorf("failed to get vmss"),
					},
					{
						desc:      "should return vmss from get",
						vmss:      &compute.VirtualMachineScaleSet{Name: to.Ptr("testVMSS")},
						expectGet: true,
					},
				}
				for i, c := range tests {
					az = getMockAzureManager(gomock.NewController(GinkgoT()))
					r = &GatewayVMConfigurationReconciler{AzureManager: az}
					mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
					if c.expectGet {
						vmConfig.Spec.GatewayNodepoolName = ""
						mockVMSSClient.EXPECT().Get(gomock.Any(), vmssRG, vmssName, gomock.Any()).Return(c.vmss, c.returnedErr)
					} else {
						vmConfig.Spec.GatewayNodepoolName = "testgw"
						mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{
							c.vmss,
						}, c.returnedErr)
					}
					foundVmss, len, err := r.getGatewayVMSS(vmConfig)
					if c.expectedErr != nil {
						Expect(err).To(Equal(c.expectedErr), "TestCase[%d]: %s", i, c.desc)
					} else {
						Expect(to.Val(foundVmss)).To(Equal(to.Val(c.vmss)), "TestCase[%d]: %s", i, c.desc)
						Expect(len).To(Equal(int32(31)), "TestCase[%d]: %s", i, c.desc)
						Expect(err).To(BeNil(), "TestCase[%d]: %s", i, c.desc)
					}
				}
			})
		})

		Context("TestEnsurePublicIPPrefix", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				r = &GatewayVMConfigurationReconciler{AzureManager: az}
				vmConfig.Spec.PublicIpPrefixId = ""
			})

			It("should return error if prefix ID provided is not valid", func() {
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/sub1"
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(err).To(Equal(fmt.Errorf("failed to parse public ip prefix id: /subscriptions/sub1")))
			})

			It("should return error if prefix ID provided is not in the same subscription", func() {
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/sub1/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(err).To(Equal(fmt.Errorf("public ip prefix subscription(sub1) is not in the same subscription(testSub)")))
			})

			It("should return error if getting prefix returns error", func() {
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(nil, fmt.Errorf("prefix not found"))
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("prefix not found")))
			})

			It("should return error if prefix returned does not have properties", func() {
				prefix := &network.PublicIPPrefix{Name: to.Ptr("prefix")}
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(prefix, nil)
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(err).To(Equal(fmt.Errorf("public ip prefix(/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix) has empty properties")))
			})

			It("should return error if prefix returned does not have expected ip prefix length", func() {
				prefix := &network.PublicIPPrefix{
					Name:       to.Ptr("prefix"),
					Properties: &network.PublicIPPrefixPropertiesFormat{PrefixLength: to.Ptr(int32(30))},
				}
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(prefix, nil)
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(err).To(Equal(fmt.Errorf("provided public ip prefix has invalid length(30), required(31)")))
			})

			It("should return valid provided public ip prefix", func() {
				prefix := &network.PublicIPPrefix{
					Name: to.Ptr("prefix"),
					ID:   to.Ptr("prefix"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength: to.Ptr(int32(31)),
						IPPrefix:     to.Ptr("1.2.3.4/31"),
					},
				}
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(prefix, nil)
				foundPrefix, prefixID, isManaged, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(foundPrefix).To(Equal("1.2.3.4/31"))
				Expect(prefixID).To(Equal(to.Val(prefix.ID)))
				Expect(isManaged).NotTo(BeTrue())
				Expect(err).To(BeNil())
			})

			It("should return error when getting managed ip prefix returns error", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should return error if managed prefix does not have properties", func() {
				prefix := &network.PublicIPPrefix{Name: to.Ptr("prefix")}
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(prefix, nil)
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(err).To(Equal(fmt.Errorf("managed public ip prefix has empty properties")))
			})

			It("should return valid managed public ip prefix", func() {
				prefix := &network.PublicIPPrefix{
					Name: to.Ptr("prefix"),
					ID:   to.Ptr("managed"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength: to.Ptr(int32(31)),
						IPPrefix:     to.Ptr("1.2.3.4/31"),
					},
				}
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(prefix, nil)
				foundPrefix, prefixID, isManaged, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(foundPrefix).To(Equal("1.2.3.4/31"))
				Expect(prefixID).To(Equal("managed"))
				Expect(isManaged).To(BeTrue())
				Expect(err).To(BeNil())
			})

			It("should create a managed public ip prefix", func() {
				expectedPrefix := &network.PublicIPPrefix{
					Name:     to.Ptr("testns_test"),
					Location: to.Ptr("location"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength:           to.Ptr(int32(31)),
						PublicIPAddressVersion: to.Ptr(network.IPVersionIPv4),
					},
					SKU: &network.PublicIPPrefixSKU{
						Name: to.Ptr(network.PublicIPPrefixSKUNameStandard),
						Tier: to.Ptr(network.PublicIPPrefixSKUTierRegional),
					},
				}
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, &azcore.ResponseError{StatusCode: http.StatusNotFound})
				mockPublicIPPrefixClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, "testns_test", gomock.Any()).DoAndReturn(
					func(ctx context.Context, resourceGroupName string, publicIPPrefixName string, ipPrefix network.PublicIPPrefix) (*network.PublicIPPrefix, error) {
						Expect(equality.Semantic.DeepEqual(ipPrefix, *expectedPrefix)).To(BeTrue())
						expectedPrefix.ID = to.Ptr("managed")
						expectedPrefix.Properties.IPPrefix = to.Ptr("1.2.3.4/31")
						return expectedPrefix, nil
					})
				foundPrefix, prefixID, isManaged, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(foundPrefix).To(Equal("1.2.3.4/31"))
				Expect(prefixID).To(Equal("managed"))
				Expect(isManaged).To(BeTrue())
				Expect(err).To(BeNil())
			})

			It("should return error when creating failed", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, &azcore.ResponseError{StatusCode: http.StatusNotFound})
				mockPublicIPPrefixClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				_, _, _, err := r.ensurePublicIPPrefix(context.TODO(), 31, vmConfig)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})
		})

		Context("TestEnsurePublicIPPrefixDeleted", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				r = &GatewayVMConfigurationReconciler{AzureManager: az}
			})

			It("should return error when getting managed ip prefix returns error", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				err := r.ensurePublicIPPrefixDeleted(context.TODO(), vmConfig)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should do nothing when managed ip prefix is not found", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, &azcore.ResponseError{StatusCode: http.StatusNotFound})
				err := r.ensurePublicIPPrefixDeleted(context.TODO(), vmConfig)
				Expect(err).To(BeNil())
			})

			It("should return error when deleting ip prefix fails", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(&network.PublicIPPrefix{}, nil)
				mockPublicIPPrefixClient.EXPECT().Delete(gomock.Any(), testRG, "testns_test").Return(fmt.Errorf("failed"))
				err := r.ensurePublicIPPrefixDeleted(context.TODO(), vmConfig)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should delete managed ip prefix", func() {
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(&network.PublicIPPrefix{}, nil)
				mockPublicIPPrefixClient.EXPECT().Delete(gomock.Any(), testRG, "testns_test").Return(nil)
				err := r.ensurePublicIPPrefixDeleted(context.TODO(), vmConfig)
				Expect(err).To(BeNil())
			})
		})

		Context("TestDifferent", func() {
			It("should detect differences between two ipConfigs properly", func() {
				tests := []struct {
					desc      string
					ipConfig1 *compute.VirtualMachineScaleSetIPConfiguration
					ipConfig2 *compute.VirtualMachineScaleSetIPConfiguration
					same      bool
				}{
					{
						desc:      "two ipConfigs without properties should be equal",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{},
						same:      true,
					},
					{
						desc:      "should return true if only one ipConfig has properties",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{}},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{},
					},
					{
						desc: "should return true if only on ipConfig is primary",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{Primary: to.Ptr(true)},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{Primary: to.Ptr(false)},
						},
					},
					{
						desc: "should return true if only one ipConfig does not have subnet",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{Subnet: &compute.APIEntityReference{}},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{},
						},
					},
					{
						desc: "should return true if two ipConfigs have different subnets",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{Subnet: &compute.APIEntityReference{ID: to.Ptr("123")}},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{Subnet: &compute.APIEntityReference{ID: to.Ptr("456")}},
						},
					},
					{
						desc: "should return true if only one ipConfig does not have public ip configuration",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{}},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{},
						},
					},
					{
						desc: "should return true if two ipConfigs have different public ip config names",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{Name: to.Ptr("123")},
							},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{Name: to.Ptr("456")},
							},
						},
					},
					{
						desc: "should return true if only one ipConfig has public ip config properties",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
									Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{},
								},
							},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{},
							},
						},
					},
					{
						desc: "should return true if only one ipConfig has public ip prefix",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
									Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
										PublicIPPrefix: &compute.SubResource{},
									},
								},
							},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
									Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{},
								},
							},
						},
					},
					{
						desc: "should return true if only one ipConfig has different public ip prefix ID",
						ipConfig1: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
									Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
										PublicIPPrefix: &compute.SubResource{ID: to.Ptr("123")},
									},
								},
							},
						},
						ipConfig2: &compute.VirtualMachineScaleSetIPConfiguration{
							Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
								PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
									Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
										PublicIPPrefix: &compute.SubResource{ID: to.Ptr("456")},
									},
								},
							},
						},
					},
				}
				for i, c := range tests {
					diff := different(c.ipConfig1, c.ipConfig2)
					Expect(diff).NotTo(Equal(c.same), "TestCase[%d]: %s", i, c.desc)
				}
			})
		})

		Context("TestReconcileVMSS", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				r = &GatewayVMConfigurationReconciler{AzureManager: az}
			})

			It("should return error if vmss does not have properties", func() {
				existingVMSS := &compute.VirtualMachineScaleSet{}
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(err).To(Equal(fmt.Errorf("vmss has empty network profile")))
			})

			It("should return error if vmss does not have primary nic", func() {
				existingVMSS := &compute.VirtualMachineScaleSet{
					Properties: &compute.VirtualMachineScaleSetProperties{
						VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
							NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
								NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{},
							},
						},
					},
				}
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("vmss(vm) primary network interface not found")))
			})

			It("should return error if updating vmss fails", func() {
				existingVMSS := getEmptyVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).Return(nil, fmt.Errorf("failed"))
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should return error if listing vmss instances fails", func() {
				existingVMSS := getEmptyVMSS()
				expectedVMSS := getConfiguredVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).Return(expectedVMSS, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(nil, fmt.Errorf("failed"))
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should return error if vmss instance has empty properties", func() {
				existingVMSS := getEmptyVMSS()
				expectedVMSS := getConfiguredVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).Return(expectedVMSS, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				vms := []*compute.VirtualMachineScaleSetVM{&compute.VirtualMachineScaleSetVM{InstanceID: to.Ptr("0")}}
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(err).To(Equal(fmt.Errorf("vmss vm(0) has empty network profile")))
			})

			It("should return error if vmss instance does not have primary nic", func() {
				existingVMSS := getEmptyVMSS()
				expectedVMSS := getConfiguredVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).Return(expectedVMSS, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				vms := []*compute.VirtualMachineScaleSetVM{&compute.VirtualMachineScaleSetVM{
					InstanceID: to.Ptr("0"),
					Properties: &compute.VirtualMachineScaleSetVMProperties{
						NetworkProfileConfiguration: &compute.VirtualMachineScaleSetVMNetworkProfileConfiguration{
							NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{},
						},
					},
				}}
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("vmss(vm) primary network interface not found")))
			})

			It("should return error if updating vmss instances fails", func() {
				existingVMSS := getEmptyVMSS()
				expectedVMSS := getConfiguredVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).Return(expectedVMSS, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				vms := []*compute.VirtualMachineScaleSetVM{getEmptyVMSSVM()}
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				mockVMSSVMClient.EXPECT().Update(gomock.Any(), testRG, vmssName, "0", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(errors.Unwrap(err)).To(Equal(fmt.Errorf("failed")))
			})

			It("should create new ipConfig for vmss and vms", func() {
				existingVMSS := getEmptyVMSS()
				expectedVMSS := getConfiguredVMSS()
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName string, vmss compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
						Expect(vmss).To(Equal(to.Val(expectedVMSS)))
						expectedVMSS.Name = to.Ptr(vmssName)
						return expectedVMSS, nil
					})
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				vms := []*compute.VirtualMachineScaleSetVM{getEmptyVMSSVM()}
				expectedVM := getConfiguredVMSSVM()
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				mockVMSSVMClient.EXPECT().Update(gomock.Any(), testRG, vmssName, "0", gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName, instanceID string, vm compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
						Expect(vm).To(Equal(to.Val(expectedVM)))
						expectedVM.InstanceID = to.Ptr("0")
						return expectedVM, nil
					})
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(err).To(BeNil())
			})

			It("should not create new ipConfig for vmss and vms when they already have expected ipConfig", func() {
				existingVMSS := getConfiguredVMSS()
				existingVMSS.Name = to.Ptr(vmssName)
				existingVM := getConfiguredVMSSVM()
				existingVM.InstanceID = to.Ptr("0")
				vms := []*compute.VirtualMachineScaleSetVM{existingVM}
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(err).To(BeNil())
			})

			It("should update ipConfigs for vmss and vms when they have unexpected setup", func() {
				existingVMSS, expectedVMSS := getConfiguredVMSS(), getConfiguredVMSS()
				existingVMSS.Name = to.Ptr(vmssName)
				existingVMSS.Properties.VirtualMachineProfile.NetworkProfile.NetworkInterfaceConfigurations[0].
					Properties.IPConfigurations[1].Properties.PrivateIPAddressVersion = to.Ptr(compute.IPVersionIPv6)
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName string, vmss compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
						Expect(vmss).To(Equal(to.Val(expectedVMSS)))
						expectedVMSS.Name = to.Ptr(vmssName)
						return expectedVMSS, nil
					})
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				existingVM, expectedVM := getConfiguredVMSSVM(), getConfiguredVMSSVM()
				existingVM.InstanceID = to.Ptr("0")
				existingVM.Properties.NetworkProfileConfiguration.NetworkInterfaceConfigurations[0].
					Properties.IPConfigurations[1].Properties.PrivateIPAddressVersion = to.Ptr(compute.IPVersionIPv6)
				vms := []*compute.VirtualMachineScaleSetVM{existingVM}
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				mockVMSSVMClient.EXPECT().Update(gomock.Any(), testRG, vmssName, "0", gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName, instanceID string, vm compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
						Expect(vm).To(Equal(to.Val(expectedVM)))
						expectedVM.InstanceID = to.Ptr("0")
						return expectedVM, nil
					})
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", true)
				Expect(err).To(BeNil())
			})

			It("should drop ipConfig for vmss and vms when reconciling deletion", func() {
				existingVMSS, expectedVMSS := getConfiguredVMSS(), getEmptyVMSS()
				expectedVMSS.Name = nil
				existingVMSS.Name = to.Ptr(vmssName)
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), testRG, vmssName, gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName string, vmss compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
						Expect(vmss).To(Equal(to.Val(expectedVMSS)))
						return expectedVMSS, nil
					})
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				existingVM, expectedVM := getConfiguredVMSSVM(), getEmptyVMSSVM()
				existingVM.InstanceID = to.Ptr("0")
				expectedVM.InstanceID = nil
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return([]*compute.VirtualMachineScaleSetVM{existingVM}, nil)
				mockVMSSVMClient.EXPECT().Update(gomock.Any(), testRG, vmssName, "0", gomock.Any()).
					DoAndReturn(func(ctx context.Context, rg, vmssName, instanceID string, vm compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
						Expect(vm).To(Equal(to.Val(expectedVM)))
						return expectedVM, nil
					})
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", false)
				Expect(err).To(BeNil())
			})

			It("should do nothing if vmss and vm does not have ipConfig when reconciling deletion", func() {
				existingVMSS := getEmptyVMSS()
				existingVM := getEmptyVMSSVM()
				vms := []*compute.VirtualMachineScaleSetVM{existingVM}
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(vms, nil)
				err := r.reconcileVMSS(context.TODO(), vmConfig, existingVMSS, "prefix", false)
				Expect(err).To(BeNil())
			})
		})

		When("reconciling vmConfig with finalizer", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				controllerutil.AddFinalizer(vmConfig, consts.VMConfigFinalizerName)
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(vmConfig).Build()
				r = &GatewayVMConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
			})

			It("should report error when getGatewayVMSS fails", func() {
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error when ensurePublicIPPrefix fails", func() {
				vmss := getConfiguredVMSS()
				vmss.Name = to.Ptr(vmssName)
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error when reconcileVMSS fails", func() {
				vmss := getConfiguredVMSS()
				vmss.Name = to.Ptr(vmssName)
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				ipPrefix := &network.PublicIPPrefix{
					Name: to.Ptr("prefix"),
					ID:   to.Ptr("prefix"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength: to.Ptr(int32(31)),
						IPPrefix:     to.Ptr("1.2.3.4/31"),
					},
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(ipPrefix, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error when removing managed public ip prefix fails", func() {
				vmss := getConfiguredVMSS()
				vmss.Name = to.Ptr(vmssName)
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				ipPrefix := &network.PublicIPPrefix{
					Name: to.Ptr("prefix"),
					ID:   to.Ptr("prefix"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength: to.Ptr(int32(31)),
						IPPrefix:     to.Ptr("1.2.3.4/31"),
					},
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(ipPrefix, nil)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return([]*compute.VirtualMachineScaleSetVM{}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should update vmConfig with public ip prefix", func() {
				vmss := getConfiguredVMSS()
				vmss.Name = to.Ptr(vmssName)
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				ipPrefix := &network.PublicIPPrefix{
					Name: to.Ptr("prefix"),
					ID:   to.Ptr("prefix"),
					Properties: &network.PublicIPPrefixPropertiesFormat{
						PrefixLength: to.Ptr(int32(31)),
						IPPrefix:     to.Ptr("1.2.3.4/31"),
					},
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(ipPrefix, nil)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, &azcore.ResponseError{StatusCode: http.StatusNotFound})
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return([]*compute.VirtualMachineScaleSetVM{}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundVMConfig)
				Expect(getErr).To(BeNil())
				Expect(foundVMConfig.Status.EgressIpPrefix).To(Equal("1.2.3.4/31"))
			})
		})

		When("deleting vmConfig", func() {
			It("should do nothing if vmConfig does not finalizer", func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				vmConfig.ObjectMeta.DeletionTimestamp = to.Ptr(metav1.Now())
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(vmConfig).Build()
				r = &GatewayVMConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundVMConfig)
				Expect(getErr).To(BeNil())
			})
		})

		When("deleting vmConfig with finalizer", func() {
			BeforeEach(func() {
				az = getMockAzureManager(gomock.NewController(GinkgoT()))
				vmConfig.Spec.PublicIpPrefixId = "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
				vmConfig.ObjectMeta.DeletionTimestamp = to.Ptr(metav1.Now())
				controllerutil.AddFinalizer(vmConfig, consts.VMConfigFinalizerName)
				cl = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(vmConfig).Build()
				r = &GatewayVMConfigurationReconciler{Client: cl, Scheme: s, AzureManager: az}
			})

			It("should report error when getGatewayVMSS fails", func() {
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(Equal(fmt.Errorf("failed")))
				getErr = getResource(cl, foundVMConfig)
				Expect(getErr).To(BeNil())
			})

			It("should report error when reconcileVMSS fails", func() {
				vmss := getEmptyVMSS()
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return(nil, fmt.Errorf("failed"))
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should report error when removing managed public ip prefix fails", func() {
				vmss := getEmptyVMSS()
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, fmt.Errorf("failed"))
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return([]*compute.VirtualMachineScaleSetVM{}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(errors.Unwrap(reconcileErr)).To(Equal(fmt.Errorf("failed")))
			})

			It("should delete vmConfig", func() {
				vmss := getEmptyVMSS()
				vmss.Tags = map[string]*string{
					consts.AKSNodepoolTagKey:             to.Ptr("testgw"),
					consts.AKSNodepoolIPPrefixSizeTagKey: to.Ptr("31"),
				}
				mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
				mockVMSSClient.EXPECT().List(gomock.Any(), testRG).Return([]*compute.VirtualMachineScaleSet{vmss}, nil)
				mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
				mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), testRG, "testns_test", gomock.Any()).Return(nil, &azcore.ResponseError{StatusCode: http.StatusNotFound})
				mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
				mockVMSSVMClient.EXPECT().List(gomock.Any(), testRG, vmssName).Return([]*compute.VirtualMachineScaleSetVM{}, nil)
				_, reconcileErr = r.Reconcile(context.TODO(), req)
				Expect(reconcileErr).To(BeNil())
				getErr = getResource(cl, foundVMConfig)
				Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			})
		})
	})
})

func getEmptyVMSS() *compute.VirtualMachineScaleSet {
	return &compute.VirtualMachineScaleSet{
		Name:     to.Ptr(vmssName),
		Location: to.Ptr("location"),
		Properties: &compute.VirtualMachineScaleSetProperties{
			VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
				NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
					NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{
						{
							Properties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
								Primary: to.Ptr(true),
								IPConfigurations: []*compute.VirtualMachineScaleSetIPConfiguration{
									{
										Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
											Primary: to.Ptr(true),
											Subnet:  &compute.APIEntityReference{ID: to.Ptr("subnet")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func getEmptyVMSSVM() *compute.VirtualMachineScaleSetVM {
	return &compute.VirtualMachineScaleSetVM{
		InstanceID: to.Ptr("0"),
		Properties: &compute.VirtualMachineScaleSetVMProperties{
			NetworkProfileConfiguration: &compute.VirtualMachineScaleSetVMNetworkProfileConfiguration{
				NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{
					{
						Properties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
							Primary: to.Ptr(true),
							IPConfigurations: []*compute.VirtualMachineScaleSetIPConfiguration{
								{
									Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
										Primary: to.Ptr(true),
										Subnet:  &compute.APIEntityReference{ID: to.Ptr("subnet")},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func getConfiguredVMSS() *compute.VirtualMachineScaleSet {
	return &compute.VirtualMachineScaleSet{
		Location: to.Ptr("location"),
		Properties: &compute.VirtualMachineScaleSetProperties{
			VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
				NetworkProfile: &compute.VirtualMachineScaleSetNetworkProfile{
					NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{
						{
							Properties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
								Primary: to.Ptr(true),
								IPConfigurations: []*compute.VirtualMachineScaleSetIPConfiguration{
									{
										Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
											Primary: to.Ptr(true),
											Subnet:  &compute.APIEntityReference{ID: to.Ptr("subnet")},
										},
									},
									{
										Name: to.Ptr("testns_test"),
										Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
											Primary:                 to.Ptr(false),
											PrivateIPAddressVersion: to.Ptr(compute.IPVersionIPv4),
											PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
												Name: to.Ptr("testns_test"),
												Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
													PublicIPPrefix: &compute.SubResource{
														ID: to.Ptr("prefix"),
													},
												},
											},
											Subnet: &compute.APIEntityReference{ID: to.Ptr("subnet")},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func getConfiguredVMSSVM() *compute.VirtualMachineScaleSetVM {
	return &compute.VirtualMachineScaleSetVM{
		Properties: &compute.VirtualMachineScaleSetVMProperties{
			NetworkProfileConfiguration: &compute.VirtualMachineScaleSetVMNetworkProfileConfiguration{
				NetworkInterfaceConfigurations: []*compute.VirtualMachineScaleSetNetworkConfiguration{
					{
						Properties: &compute.VirtualMachineScaleSetNetworkConfigurationProperties{
							Primary: to.Ptr(true),
							IPConfigurations: []*compute.VirtualMachineScaleSetIPConfiguration{
								{
									Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
										Primary: to.Ptr(true),
										Subnet:  &compute.APIEntityReference{ID: to.Ptr("subnet")},
									},
								},
								{
									Name: to.Ptr("testns_test"),
									Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
										Primary:                 to.Ptr(false),
										PrivateIPAddressVersion: to.Ptr(compute.IPVersionIPv4),
										PublicIPAddressConfiguration: &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
											Name: to.Ptr("testns_test"),
											Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
												PublicIPPrefix: &compute.SubResource{
													ID: to.Ptr("prefix"),
												},
											},
										},
										Subnet: &compute.APIEntityReference{ID: to.Ptr("subnet")},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
