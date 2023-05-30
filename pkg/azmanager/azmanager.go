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
	"context"
	"fmt"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v3"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/interfaceclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetvmclient"

	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

const (
	DefaultUserAgent = "kube-egress-gateway-controller"

	// LB frontendIPConfiguration ID template
	LBFrontendIPConfigTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s"
	// LB backendAddressPool ID template
	LBBackendPoolIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s"
	// LB probe ID template
	LBProbeIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s"
)

type AzureManager struct {
	*config.CloudConfig

	LoadBalancerClient   loadbalancerclient.Interface
	VmssClient           virtualmachinescalesetclient.Interface
	VmssVMClient         virtualmachinescalesetvmclient.Interface
	PublicIPPrefixClient publicipprefixclient.Interface
	InterfaceClient      interfaceclient.Interface
	SubnetClient         subnetclient.Interface
}

func CreateAzureManager(cloud *config.CloudConfig, factory azureclients.AzureClientsFactory) (*AzureManager, error) {
	az := AzureManager{
		CloudConfig: cloud,
	}

	if az.UserAgent == "" {
		az.UserAgent = DefaultUserAgent
	}

	if az.LoadBalancerResourceGroup == "" {
		az.LoadBalancerResourceGroup = az.ResourceGroup
	}

	if az.VnetResourceGroup == "" {
		az.VnetResourceGroup = az.ResourceGroup
	}

	lbClient, err := factory.GetLoadBalancersClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.LoadBalancerClient = lbClient

	vmssClient, err := factory.GetVirtualMachineScaleSetsClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.VmssClient = vmssClient

	publicIPPrefixClient, err := factory.GetPublicIPPrefixesClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.PublicIPPrefixClient = publicIPPrefixClient

	vmssVMClient, err := factory.GetVirtualMachineScaleSetVMsClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.VmssVMClient = vmssVMClient

	interfaceClient, err := factory.GetInterfacesClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.InterfaceClient = interfaceClient

	subnetClient, err := factory.GetSubnetsClient()
	if err != nil {
		return &AzureManager{}, err
	}
	az.SubnetClient = subnetClient

	return &az, nil
}

func (az *AzureManager) SubscriptionID() string {
	return az.CloudConfig.SubscriptionID
}

func (az *AzureManager) Location() string {
	return az.CloudConfig.Location
}

func (az *AzureManager) LoadBalancerName() string {
	return az.CloudConfig.LoadBalancerName
}

func (az *AzureManager) GetLBFrontendIPConfigurationID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBFrontendIPConfigTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLBBackendAddressPoolID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBBackendPoolIDTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLBProbeID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBProbeIDTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLB() (*network.LoadBalancer, error) {
	lb, err := az.LoadBalancerClient.Get(context.Background(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), nil)
	if err != nil {
		return nil, err
	}
	return lb, nil
}

func (az *AzureManager) CreateOrUpdateLB(lb network.LoadBalancer) (*network.LoadBalancer, error) {
	ret, err := az.LoadBalancerClient.CreateOrUpdate(context.Background(), az.LoadBalancerResourceGroup, to.Val(lb.Name), lb)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (az *AzureManager) DeleteLB() error {
	if err := az.LoadBalancerClient.Delete(context.Background(), az.LoadBalancerResourceGroup, az.LoadBalancerName()); err != nil {
		return err
	}
	return nil
}

func (az *AzureManager) ListVMSS() ([]*compute.VirtualMachineScaleSet, error) {
	vmssList, err := az.VmssClient.List(context.Background(), az.ResourceGroup)
	if err != nil {
		return nil, err
	}
	return vmssList, nil
}

func (az *AzureManager) GetVMSS(resourceGroup, vmssName string) (*compute.VirtualMachineScaleSet, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	vmss, err := az.VmssClient.Get(context.Background(), resourceGroup, vmssName)
	if err != nil {
		return nil, err
	}
	return vmss, nil
}

func (az *AzureManager) CreateOrUpdateVMSS(resourceGroup, vmssName string, vmss compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	retVmss, err := az.VmssClient.CreateOrUpdate(context.Background(), resourceGroup, vmssName, vmss)
	if err != nil {
		return nil, err
	}
	return retVmss, nil
}

func (az *AzureManager) ListVMSSInstances(resourceGroup, vmssName string) ([]*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	vms, err := az.VmssVMClient.List(context.Background(), resourceGroup, vmssName)
	if err != nil {
		return nil, err
	}
	return vms, nil
}

func (az *AzureManager) GetVMSSInstance(resourceGroup, vmssName, instanceID string) (*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("vmss instanceID is empty")
	}
	vm, err := az.VmssVMClient.Get(context.Background(), resourceGroup, vmssName, instanceID)
	if err != nil {
		return nil, err
	}
	return vm, nil
}

func (az *AzureManager) UpdateVMSSInstance(resourceGroup, vmssName, instanceID string, vm compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("vmss instanceID is empty")
	}
	retVM, err := az.VmssVMClient.Update(context.Background(), resourceGroup, vmssName, instanceID, vm)
	if err != nil {
		return nil, err
	}
	return retVM, nil
}

func (az *AzureManager) GetPublicIPPrefix(resourceGroup, prefixName string) (*network.PublicIPPrefix, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return nil, fmt.Errorf("public ip prefix name is empty")
	}
	prefix, err := az.PublicIPPrefixClient.Get(context.Background(), resourceGroup, prefixName, nil)
	if err != nil {
		return nil, err
	}
	return prefix, nil
}

func (az *AzureManager) CreateOrUpdatePublicIPPrefix(resourceGroup, prefixName string, ipPrefix network.PublicIPPrefix) (*network.PublicIPPrefix, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return nil, fmt.Errorf("public ip prefix name is empty")
	}
	prefix, err := az.PublicIPPrefixClient.CreateOrUpdate(context.Background(), resourceGroup, prefixName, ipPrefix)
	if err != nil {
		return nil, err
	}
	return prefix, nil
}

func (az *AzureManager) DeletePublicIPPrefix(resourceGroup, prefixName string) error {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return fmt.Errorf("public ip prefix name is empty")
	}
	return az.PublicIPPrefixClient.Delete(context.Background(), resourceGroup, prefixName)
}

func (az *AzureManager) GetVMSSInterface(resourceGroup, vmssName, instanceID, interfaceName string) (*network.Interface, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID is empty")
	}
	if interfaceName == "" {
		return nil, fmt.Errorf("interface name is empty")
	}
	nicResp, err := az.InterfaceClient.GetVirtualMachineScaleSetNetworkInterface(context.Background(), resourceGroup, vmssName, instanceID, interfaceName, nil)
	if err != nil {
		return nil, err
	}
	return &nicResp.Interface, nil
}

func (az *AzureManager) GetSubnet() (*network.Subnet, error) {
	subnet, err := az.SubnetClient.Get(context.Background(), az.VnetResourceGroup, az.VnetName, az.SubnetName, nil)
	if err != nil {
		return nil, err
	}
	return subnet, nil
}
