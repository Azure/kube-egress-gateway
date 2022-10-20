package azmanager

import (
	"context"
	"fmt"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient"
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

	LoadBalancerClient loadbalancerclient.Interface
	VmssClient         vmssclient.Interface
}

func CreateAzureManager(cloud *config.CloudConfig, factory azureclients.AzureClientsFactory) (AzureManager, error) {
	az := AzureManager{
		CloudConfig: cloud,
	}

	if az.UserAgent == "" {
		az.UserAgent = DefaultUserAgent
	}

	if az.VnetResourceGroup == "" {
		az.VnetResourceGroup = az.ResourceGroup
	}

	if az.LoadBalancerResourceGroup == "" {
		az.LoadBalancerResourceGroup = az.ResourceGroup
	}

	lbClient, err := factory.GetLoadBalancersClient()
	if err != nil {
		return AzureManager{}, err
	}
	az.LoadBalancerClient = lbClient
	vmssClient, err := factory.GetVirtualMachineScaleSetsClient()
	if err != nil {
		return AzureManager{}, err
	}
	az.VmssClient = vmssClient

	return az, nil
}

func (az *AzureManager) GetLBFrontendIPConfigurationID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBFrontendIPConfigTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLBBackendAddressPoolID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBBackendPoolIDTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLBProbeID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBProbeIDTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLB() (*network.LoadBalancer, error) {
	lb, err := az.LoadBalancerClient.Get(context.Background(), az.LoadBalancerResourceGroup, az.LoadBalancerName, "")
	if err != nil {
		return nil, err
	}
	return lb, nil
}

func (az *AzureManager) CreateOrUpdateLB(lb network.LoadBalancer) error {
	if _, err := az.LoadBalancerClient.CreateOrUpdate(context.Background(), az.LoadBalancerResourceGroup, to.Val(lb.Name), lb); err != nil {
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
	vmss, err := az.VmssClient.Get(context.Background(), resourceGroup, vmssName, "")
	if err != nil {
		return nil, err
	}
	return vmss, nil
}
