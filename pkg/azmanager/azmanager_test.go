// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package azmanager

import (
	"context"
	"fmt"
	"testing"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/interfaceclient/mock_interfaceclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient/mock_loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/mock_azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient/mock_publicipprefixclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient/mock_subnetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient/mock_virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetvmclient/mock_virtualmachinescalesetvmclient"

	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

func TestCreateAzureManager(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	config := getTestCloudConfig()
	factory := getMockFactory(ctrl)
	az, err := CreateAzureManager(config, factory)
	assert.Nil(t, err)
	assert.Equal(t, az.SubscriptionID(), config.SubscriptionID)
	assert.Equal(t, az.Location(), config.Location)
}

func TestGets(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	config := getTestCloudConfig()
	factory := getMockFactory(ctrl)
	az, err := CreateAzureManager(config, factory)
	assert.Nil(t, err, "CreateAzureManager() should not return error")
	expectedFrontendID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, expectedFrontendID, to.Val(az.GetLBFrontendIPConfigurationID("test")), "GetLBFrontendIPConfigurationID() should return expected result")
	expectedLBBackendAddressPoolID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, expectedLBBackendAddressPoolID, to.Val(az.GetLBBackendAddressPoolID("test")), "GetLBBackendAddressPoolID() should return expected result")
	expectedLBProbeID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s",
		config.SubscriptionID, config.LoadBalancerResourceGroup, config.LoadBalancerName, "test")
	assert.Equal(t, expectedLBProbeID, to.Val(az.GetLBProbeID("test")), "GetLBProbeID() should return expected result")

	assert.Equal(t, "testLB", az.LoadBalancerName(), "LoadBalancerName() should return loadBalancer name from config")
	az.CloudConfig.LoadBalancerName = ""
	assert.Equal(t, consts.DefaultGatewayLBName, az.LoadBalancerName(), "LoadBalancerName() should return default loadBalancer name if it's empty in config")
}

func TestGetLB(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().Get(gomock.Any(), "testRG", "testLB", gomock.Any()).Return(test.lb, test.testErr)
		lb, err := az.GetLB(context.Background())
		assert.Equal(t, to.Val(lb), to.Val(test.lb), "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestCreateOrUpdateLB(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().CreateOrUpdate(gomock.Any(), "testRG", "testLB", to.Val(test.lb)).Return(test.lb, test.testErr)
		ret, err := az.CreateOrUpdateLB(context.Background(), to.Val(test.lb))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		if test.testErr == nil {
			assert.Equal(t, to.Val(test.lb), to.Val(ret), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestDeleteLB(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockLoadBalancerClient := az.LoadBalancerClient.(*mock_loadbalancerclient.MockInterface)
		mockLoadBalancerClient.EXPECT().Delete(gomock.Any(), "testRG", "testLB").Return(test.testErr)
		err := az.DeleteLB(context.Background())
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestListVMSS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc     string
		vmssList []*compute.VirtualMachineScaleSet
		testErr  error
	}{
		{
			desc:     "ListVMSS() should return expected vmss list",
			vmssList: []*compute.VirtualMachineScaleSet{{Name: to.Ptr("vmss")}},
		},
		{
			desc:    "ListVMSS() should return expected error",
			testErr: fmt.Errorf("failed to list vmss"),
		},
	}
	for i, test := range tests {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
		mockVMSSClient.EXPECT().List(gomock.Any(), "testRG").Return(test.vmssList, test.testErr)
		vmssList, err := az.ListVMSS(context.Background())
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, len(vmssList), len(test.vmssList), "TestCase[%d]: %s", i, test.desc)
		for j, vmss := range vmssList {
			assert.Equal(t, to.Val(vmss), to.Val(test.vmssList[j]), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestGetVMSS(t *testing.T) {
	t.Parallel()

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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
			mockVMSSClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.vmssName, gomock.Any()).Return(test.vmss, test.testErr)
		}
		vmss, err := az.GetVMSS(context.Background(), test.rg, test.vmssName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vmss), to.Val(test.vmss), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetVMSSWithRateLimitError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	config := getTestCloudConfig()
	factory := getMockFactory(ctrl)
	az, _ := CreateAzureManager(config, factory)
	mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
	mockVMSSClient.EXPECT().Get(gomock.Any(), "testRG", "vmss", gomock.Any()).Return(nil, fmt.Errorf("rate limit reached")).Times(1)
	mockVMSSClient.EXPECT().Get(gomock.Any(), "testRG", "vmss", gomock.Any()).Return(&compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")}, nil)

	vmss, err := az.GetVMSS(context.Background(), "testRG", "vmss")
	assert.NoError(t, err)
	assert.Equal(t, to.Val(vmss), to.Val(&compute.VirtualMachineScaleSet{Name: to.Ptr("vmss")}))
}

func TestCreateOrUpdateVMSS(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSClient := az.VmssClient.(*mock_virtualmachinescalesetclient.MockInterface)
			mockVMSSClient.EXPECT().CreateOrUpdate(gomock.Any(), test.expectedRG, test.vmssName, to.Val(test.vmss)).Return(test.vmss, test.testErr)
		}
		vmss, err := az.CreateOrUpdateVMSS(context.Background(), test.rg, test.vmssName, to.Val(test.vmss))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vmss), to.Val(test.vmss), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestListVMSSInstances(t *testing.T) {
	t.Parallel()
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
			vms:          []*compute.VirtualMachineScaleSetVM{{Name: to.Ptr("vm0")}},
			expectedCall: true,
		},
		{
			desc:         "ListVMSSInstances() should return expected vmss instances with specified resource group",
			rg:           "customRG",
			expectedRG:   "customRG",
			vmssName:     "vmss",
			vms:          []*compute.VirtualMachineScaleSetVM{{Name: to.Ptr("vm0")}},
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mock_virtualmachinescalesetvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().List(gomock.Any(), test.expectedRG, test.vmssName).Return(test.vms, test.testErr)
		}
		vms, err := az.ListVMSSInstances(context.Background(), test.rg, test.vmssName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, len(vms), len(test.vms), "TestCase[%d]: %s", i, test.desc)
		for j, vm := range vms {
			assert.Equal(t, to.Val(vm), to.Val(test.vms[j]), "TestCase[%d]: %s", i, test.desc)
		}
	}
}

func TestGetVMSSInstance(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mock_virtualmachinescalesetvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID).Return(test.vm, test.testErr)
		}
		vm, err := az.GetVMSSInstance(context.Background(), test.rg, test.vmssName, test.instanceID)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vm), to.Val(test.vm), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestUpdateVMSSInstance(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockVMSSVMClient := az.VmssVMClient.(*mock_virtualmachinescalesetvmclient.MockInterface)
			mockVMSSVMClient.EXPECT().Update(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, to.Val(test.vm)).Return(test.vm, test.testErr)
		}
		vm, err := az.UpdateVMSSInstance(context.Background(), test.rg, test.vmssName, test.instanceID, to.Val(test.vm))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(vm), to.Val(test.vm), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetPublicIPPrefix(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mock_publicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().Get(gomock.Any(), test.expectedRG, test.prefixName, gomock.Any()).Return(test.prefix, test.testErr)
		}
		prefix, err := az.GetPublicIPPrefix(context.Background(), test.rg, test.prefixName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(prefix), to.Val(test.prefix), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestCreateOrUpdatePublicIPPrefix(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mock_publicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().CreateOrUpdate(gomock.Any(), test.expectedRG, test.prefixName, to.Val(test.prefix)).Return(test.prefix, test.testErr)
		}
		prefix, err := az.CreateOrUpdatePublicIPPrefix(context.Background(), test.rg, test.prefixName, to.Val(test.prefix))
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(prefix), to.Val(test.prefix), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestDeletePublicIPPrefix(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockPublicIPPrefixClient := az.PublicIPPrefixClient.(*mock_publicipprefixclient.MockInterface)
			mockPublicIPPrefixClient.EXPECT().Delete(gomock.Any(), test.expectedRG, test.prefixName).Return(test.testErr)
		}
		err := az.DeletePublicIPPrefix(context.Background(), test.rg, test.prefixName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetVMSSInterface(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		if test.expectedCall {
			mockInterfaceClient := az.InterfaceClient.(*mock_interfaceclient.MockInterface)
			if test.nic != nil {
				mockInterfaceClient.EXPECT().GetVirtualMachineScaleSetNetworkInterface(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, test.nicName).Return(test.nic, test.testErr)
			} else {
				mockInterfaceClient.EXPECT().GetVirtualMachineScaleSetNetworkInterface(gomock.Any(), test.expectedRG, test.vmssName, test.instanceID, test.nicName).Return(nil, test.testErr)

			}
		}
		nic, err := az.GetVMSSInterface(context.Background(), test.rg, test.vmssName, test.instanceID, test.nicName)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, to.Val(nic), to.Val(test.nic), "TestCase[%d]: %s", i, test.desc)
	}
}

func TestGetSubnet(t *testing.T) {
	t.Parallel()
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
		config := getTestCloudConfig()
		factory := getMockFactory(ctrl)
		az, _ := CreateAzureManager(config, factory)
		mockSubnetClient := az.SubnetClient.(*mock_subnetclient.MockInterface)
		mockSubnetClient.EXPECT().Get(gomock.Any(), "testRG", "testVnet", "testSubnet", gomock.Any()).Return(test.subnet, test.testErr)
		subnet, err := az.GetSubnet(context.Background())
		assert.Equal(t, to.Val(subnet), to.Val(test.subnet), "TestCase[%d]: %s", i, test.desc)
		assert.Equal(t, err, test.testErr, "TestCase[%d]: %s", i, test.desc)
	}
}

func getMockFactory(ctrl *gomock.Controller) azclient.ClientFactory {
	factory := mock_azclient.NewMockClientFactory(ctrl)
	factory.EXPECT().GetLoadBalancerClient().Return(mock_loadbalancerclient.NewMockInterface(ctrl))
	factory.EXPECT().GetVirtualMachineScaleSetClient().Return(mock_virtualmachinescalesetclient.NewMockInterface(ctrl))
	factory.EXPECT().GetVirtualMachineScaleSetVMClient().Return(mock_virtualmachinescalesetvmclient.NewMockInterface(ctrl))
	factory.EXPECT().GetPublicIPPrefixClient().Return(mock_publicipprefixclient.NewMockInterface(ctrl))
	factory.EXPECT().GetInterfaceClient().Return(mock_interfaceclient.NewMockInterface(ctrl))
	factory.EXPECT().GetSubnetClient().Return(mock_subnetclient.NewMockInterface(ctrl))
	return factory
}

func getTestCloudConfig() *config.CloudConfig {
	return &config.CloudConfig{
		ARMClientConfig: azclient.ARMClientConfig{
			Cloud: "AzureTest",
		},
		Location:                  "location",
		SubscriptionID:            "testSub",
		ResourceGroup:             "testRG",
		LoadBalancerName:          "testLB",
		LoadBalancerResourceGroup: "testRG",
		VnetName:                  "testVnet",
		SubnetName:                "testSubnet",
		VnetResourceGroup:         "testRG",
	}
}
