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
package azmanager

import (
	"fmt"
	"testing"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/interfaceclient/mockinterfaceclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient/mockloadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient/mockpublicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/subnetclient/mocksubnetclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient/mockvmssclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient/mockvmssvmclient"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestCreateAzureManager(t *testing.T) {
	tests := []struct {
		desc                      string
		userAgent                 string
		expectedUserAgent         string
		lbResourceGroup           string
		expectedLBResourceGroup   string
		vnetResourceGroup         string
		expectedVnetResourceGroup string
	}{
		{
			desc:                      "test default userAgent, lbResourceGroup and vnetResourceGroup",
			expectedUserAgent:         "kube-egress-gateway-controller",
			expectedLBResourceGroup:   "testRG",
			expectedVnetResourceGroup: "testRG",
		},
		{
			desc:                      "test custom userAgent, lbResourceGroup and vnetResourceGroup",
			userAgent:                 "testUserAgent",
			expectedUserAgent:         "testUserAgent",
			lbResourceGroup:           "testLBRG",
			expectedLBResourceGroup:   "testLBRG",
			vnetResourceGroup:         "testVnetRG",
			expectedVnetResourceGroup: "testVnetRG",
		},
	}

	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig(test.userAgent, test.lbResourceGroup, test.vnetResourceGroup)
		factory := getMockFactory(ctrl)
		az, err := CreateAzureManager(config, factory)
		assert.Nil(t, err, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, az.UserAgent, test.expectedUserAgent, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, az.LoadBalancerResourceGroup, test.expectedLBResourceGroup, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, az.VnetResourceGroup, test.expectedVnetResourceGroup, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, az.SubscriptionID(), config.SubscriptionID, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, az.Location(), config.Location, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	config := getTestCloudConfig("test", "testLBRG", "")
	factory := getMockFactory(ctrl)
	az, err := CreateAzureManager(config, factory)
	assert.Nil(t, err, "CreateAzureManager() should not return error")
	expectedFrontendID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, to.Val(az.GetLBFrontendIPConfigurationID("test")), expectedFrontendID, "GetLBFrontendIPConfigurationID() should return expected result")
	expectedLBBackendAddressPoolID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, to.Val(az.GetLBBackendAddressPoolID("test")), expectedLBBackendAddressPoolID, "GetLBBackendAddressPoolID() should return expected result")
	expectedLBProbeID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, to.Val(az.GetLBProbeID("test")), expectedLBProbeID, "GetLBProbeID() should return expected result")
}

func TestGetLB(t *testing.T) {
	tests := []struct {
		desc    string
		lb      *network.LoadBalancer
		testErr error
	}{
		{
			desc: "GetLB() should return expected LB",
			lb:   &network.LoadBalancer{Name: to.Ptr("testLB")},
		},
		{
			desc:    "GetLB() should return expected error",
			testErr: fmt.Errorf("LB not found"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mockloadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().Get(gomock.Any(), "testRG", "testLB", gomock.Any()).Return(test.lb, test.testErr)
		lb, err := az.GetLB()
		assert.Equal(t, to.Val(lb), to.Val(test.lb), "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestCreateOrUpdateLB(t *testing.T) {
	tests := []struct {
		desc    string
		lb      *network.LoadBalancer
		testErr error
	}{
		{
			desc: "CreateOrUpdateLB() should run successfully",
			lb:   &network.LoadBalancer{Name: to.Ptr("testLB")},
		},
		{
			desc:    "CreateOrUpdateLB() should return expected error",
			lb:      &network.LoadBalancer{Name: to.Ptr("testLB")},
			testErr: fmt.Errorf("failed to create lb"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mockloadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), "testRG", "testLB", to.Val(test.lb)).Return(test.lb, test.testErr)
		ret, err := az.CreateOrUpdateLB(to.Val(test.lb))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		if test.testErr == nil {
			assert.Equal(t, to.Val(test.lb), to.Val(ret), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestDeleteLB(t *testing.T) {
	tests := []struct {
		desc    string
		testErr error
	}{
		{
			desc: "DeleteLB() should run as expected",
		},
		{
			desc:    "DeleteLB() should return expected error",
			testErr: fmt.Errorf("failed to delete public ip prefix"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mockloadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().Delete(gomock.Any(), "testRG", "testLB").Return(test.testErr)
		err := az.DeleteLB()
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestListVMSS(t *testing.T) {
	tests := []struct {
		desc     string
		vmssList []*compute.VirtualMachineScaleSet
		testErr  error
	}{
		{
			desc:     "ListVMSS() should return expected vmss list",
			vmssList: []*compute.VirtualMachineScaleSet{&compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")}},
		},
		{
			desc:    "ListVMSS() should return expected error",
			testErr: fmt.Errorf("failed to list vmss"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
		mockVMSSClient.EXPECT().List(gomock.Any(), "testRG").Return(test.vmssList, test.testErr)
		vmssList, err := az.ListVMSS()
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, len(vmssList), len(test.vmssList), "TestCase[%d]: %s", i, test.desc)
		for j, vmss := range vmssList {
			assert.Equal(t, to.Val(vmss), to.Val(test.vmssList[j]), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestGetVMSS(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		vmss         *compute.VirtualMachineScaleSet
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "GetVMSS() should return expected vmss",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			vmss:         &compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSS() should return expected vmss with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			vmss:         &compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSS() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "GetVMSS() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			expectedCall: true,
			testErr:      fmt.Errorf("vmss not found"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
			mockVMSSClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.vmssName, gomock.Any()).Return(test.vmss, test.testErr)
		}
		vmss, err := az.GetVMSS(test.rg, test.vmssName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vmss), to.Val(test.vmss), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestCreateOrUpdateVMSS(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		vmss         *compute.VirtualMachineScaleSet
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "CreateOrUpdateVMSS() should run as expected",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			vmss:         &compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")},
			expectedCall: true,
		},
		{
			desc:         "CreateOrUpdateVMSS() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			vmss:         &compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")},
			expectedCall: true,
		},
		{
			desc:         "CreateOrUpdateVMSS() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "CreateOrUpdateVMSS() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to update vmss"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSClient := az.VmssClient.(*mockvmssclient.MockInterface)
			mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), test.expectedRG, test.vmssName, to.Val(test.vmss)).Return(test.vmss, test.testErr)
		}
		vmss, err := az.CreateOrUpdateVMSS(test.rg, test.vmssName, to.Val(test.vmss))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vmss), to.Val(test.vmss), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestListVMSSInstances(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		vms          []*compute.VirtualMachineScaleSetVM
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "ListVMSSInstances() should return expected vmss instances",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			vms:          []*compute.VirtualMachineScaleSetVM{&compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")}},
			expectedCall: true,
		},
		{
			desc:         "ListVMSSInstances() should return expected vmss instances with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			vms:          []*compute.VirtualMachineScaleSetVM{&compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")}},
			expectedCall: true,
		},
		{
			desc:         "ListVMSSInstances() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "ListVMSSInstances() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to list vmss instances"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().List(gomock.Any(), test.expectedRG, test.vmssName).Return(test.vms, test.testErr)
		}
		vms, err := az.ListVMSSInstances(test.rg, test.vmssName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, len(vms), len(test.vms), "TestCase[%d]: %s", i, test.desc)
		for j, vm := range vms {
			assert.Equal(t, to.Val(vm), to.Val(test.vms[j]), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestGetVMSSInstance(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		instanceID   string
		vm           *compute.VirtualMachineScaleSetVM
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "GetVMSSInstance() should run as expected",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			vm:           &compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSSInstance() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			instanceID:   "0",
			vm:           &compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSSInstance() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "GetVMSSInstance() should return error when instanceID is empty",
			vmssName:     "vmss",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss instanceID is empty"),
		},
		{
			desc:         "GetVMSSInstance() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to list vmss instances"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, gomock.Any()).Return(test.vm, test.testErr)
		}
		vm, err := az.GetVMSSInstance(test.rg, test.vmssName, test.instanceID)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vm), to.Val(test.vm), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestUpdateVMSSInstance(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		instanceID   string
		vm           *compute.VirtualMachineScaleSetVM
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "UpdateVMSSInstance() should run as expected",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			vm:           &compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")},
			expectedCall: true,
		},
		{
			desc:         "UpdateVMSSInstance() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			instanceID:   "0",
			vm:           &compute.VirtualMachineScaleSetVM{Name: to.Ptr("vm0")},
			expectedCall: true,
		},
		{
			desc:         "UpdateVMSSInstance() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "UpdateVMSSInstance() should return error when instanceID is empty",
			vmssName:     "vmss",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss instanceID is empty"),
		},
		{
			desc:         "UpdateVMSSInstance() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to list vmss instances"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mockvmssvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().Update(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, to.Val(test.vm)).Return(test.vm, test.testErr)
		}
		vm, err := az.UpdateVMSSInstance(test.rg, test.vmssName, test.instanceID, to.Val(test.vm))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vm), to.Val(test.vm), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetPublicIPPrefix(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		prefixName   string
		prefix       *network.PublicIPPrefix
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "GetPublicIPPrefix() should return expected ip prefix",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			prefix:       &network.PublicIPPrefix{Name: to.Ptr("prefix")},
			expectedCall: true,
		},
		{
			desc:         "GetPublicIPPrefix() should return ip prefix with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			prefixName:   "prefix",
			prefix:       &network.PublicIPPrefix{Name: to.Ptr("prefix")},
			expectedCall: true,
		},
		{
			desc:         "GetPublicIPPrefix() should return error when prefix name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("public ip prefix name is empty"),
		},
		{
			desc:         "GetPublicIPPrefix() should return expected error",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			expectedCall: true,
			testErr:      fmt.Errorf("public ip prefix not found"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.prefixName, gomock.Any()).Return(test.prefix, test.testErr)
		}
		prefix, err := az.GetPublicIPPrefix(test.rg, test.prefixName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(prefix), to.Val(test.prefix), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestCreateOrUpdatePublicIPPrefix(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		prefixName   string
		prefix       *network.PublicIPPrefix
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "CreateOrUpdatePublicIPPrefix() should run as expected",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			prefix:       &network.PublicIPPrefix{Name: to.Ptr("prefix")},
			expectedCall: true,
		},
		{
			desc:         "CreateOrUpdatePublicIPPrefix() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			prefixName:   "prefix",
			prefix:       &network.PublicIPPrefix{Name: to.Ptr("prefix")},
			expectedCall: true,
		},
		{
			desc:         "CreateOrUpdatePublicIPPrefix() should return error when prefix name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("public ip prefix name is empty"),
		},
		{
			desc:         "CreateOrUpdatePublicIPPrefix() should return expected error",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to create public ip prefix"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().CreateOrUpdate(gomock.Any(), test.expectedRG, test.prefixName, to.Val(test.prefix)).Return(test.prefix, test.testErr)
		}
		prefix, err := az.CreateOrUpdatePublicIPPrefix(test.rg, test.prefixName, to.Val(test.prefix))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(prefix), to.Val(test.prefix), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestDeletePublicIPPrefix(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		prefixName   string
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "DeletePublicIPPrefix() should run as expected",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			expectedCall: true,
		},
		{
			desc:         "DeletePublicIPPrefix() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			prefixName:   "prefix",
			expectedCall: true,
		},
		{
			desc:         "DeletePublicIPPrefix() should return error when prefix name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("public ip prefix name is empty"),
		},
		{
			desc:         "DeletePublicIPPrefix() should return expected error",
			expectedRG:   "testRG",
			prefixName:   "prefix",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to delete public ip prefix"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mockpublicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().Delete(gomock.Any(), test.expectedRG, test.prefixName).Return(test.testErr)
		}
		err := az.DeletePublicIPPrefix(test.rg, test.prefixName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetVMSSInterface(t *testing.T) {
	tests := []struct {
		desc         string
		rg           string
		expectedRG   string
		vmssName     string
		instanceID   string
		nicName      string
		nic          *network.Interface
		expectedCall bool
		testErr      error
	}{
		{
			desc:         "GetVMSSInterface() should run as expected",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			nicName:      "nic",
			nic:          &network.Interface{Name: to.Ptr("nic")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSSInterface() should run as expected with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			instanceID:   "0",
			nicName:      "nic",
			nic:          &network.Interface{Name: to.Ptr("nic")},
			expectedCall: true,
		},
		{
			desc:         "GetVMSSInterface() should return error when vmss name is empty",
			expectedCall: false,
			testErr:      fmt.Errorf("vmss name is empty"),
		},
		{
			desc:         "GetVMSSInterface() should return error when instanceID is empty",
			vmssName:     "vmss",
			expectedCall: false,
			testErr:      fmt.Errorf("instanceID is empty"),
		},
		{
			desc:         "GetVMSSInterface() should return error when interfaceName is empty",
			vmssName:     "vmss",
			instanceID:   "0",
			expectedCall: false,
			testErr:      fmt.Errorf("interface name is empty"),
		},
		{
			desc:         "GetVMSSInterface() should return expected error",
			expectedRG:   "testRG",
			vmssName:     "vmss",
			instanceID:   "0",
			nicName:      "nic",
			expectedCall: true,
			testErr:      fmt.Errorf("failed to list vmss instances"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockInterfaceClient := az.InterfaceClient.(*mockinterfaceclient.MockInterface)
			mockInterfaceClient.EXPECT().GetVirtualMachineScaleSetNetworkInterface(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, test.nicName, gomock.Any()).Return(test.nic, test.testErr)
		}
		nic, err := az.GetVMSSInterface(test.rg, test.vmssName, test.instanceID, test.nicName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(nic), to.Val(test.nic), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetSubnet(t *testing.T) {
	tests := []struct {
		desc    string
		subnet  *network.Subnet
		testErr error
	}{
		{
			desc:   "GetSubnet() should return expected subnet",
			subnet: &network.Subnet{Name: to.Ptr("testSubnet")},
		},
		{
			desc:    "GetSubnet() should return expected error",
			testErr: fmt.Errorf("Subnet not found"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig("", "", "")
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockSubnetClient := az.SubnetClient.(*mocksubnetclient.MockInterface)
		mockSubnetClient.EXPECT().Get(gomock.Any(), "testRG", "testVnet", "testSubnet", gomock.Any()).Return(test.subnet, test.testErr)
		subnet, err := az.GetSubnet()
		assert.Equal(t, to.Val(subnet), to.Val(test.subnet), "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func getMockFactory(ctrl *gomock.Controller) azureclients.AzureClientsFactory {
	return azureclients.NewMockAzureClientsFactory(ctrl)
}

func getTestCloudConfig(userAgent, lbRG, vnetRG string) *config.CloudConfig {
	return &config.CloudConfig{
		Cloud:                     "AzureTest",
		Location:                  "location",
		SubscriptionID:            "testSub",
		UserAgent:                 userAgent,
		ResourceGroup:             "testRG",
		LoadBalancerName:          "testLB",
		LoadBalancerResourceGroup: lbRG,
		VnetName:                  "testVnet",
		SubnetName:                "testSubnet",
		VnetResourceGroup:         vnetRG,
	}
}
