package azmanager

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-08-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/spf13/viper"

	"sigs.k8s.io/cloud-provider-azure/pkg/auth"
	azclients "sigs.k8s.io/cloud-provider-azure/pkg/azureclients"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/subnetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azureclients/vmssclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/retry"
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

type Config struct {
	auth.AzureAuthConfig

	// azure cloud
	Cloud string
	// azure resource location
	Location string
	// user agent for Azure customer usage attribution
	UserAgent string
	// default resource group where the gateway nodes are deployed
	ResourceGroup string
	// name of the virtual network where the gateway nodes and lb frontend ip belongs
	VnetName string
	// resource group where the virtual network is deployed
	VnetResourceGroup string
	// name of the subnet where the gateway nodes and lb frontend ip belongs
	SubnetName string
	// name of the gateway ILB
	LoadBalancerName string
	// resource group where the gateway ILB belongs
	LoadBalancerResourceGroup string
}

type AzureManager struct {
	Config
	Environment azure.Environment

	SubnetsClient      subnetclient.Interface
	LoadBalancerClient loadbalancerclient.Interface
	VmssClient         vmssclient.Interface
}

func CreateAzureManager() (AzureManager, error) {
	az := AzureManager{}

	if err := viper.Unmarshal(&az.AzureAuthConfig); err != nil {
		return AzureManager{}, err
	}

	if err := viper.Unmarshal(&az.Config); err != nil {
		return AzureManager{}, err
	}

	if err := az.validate(); err != nil {
		return AzureManager{}, fmt.Errorf("failed to create azure manager: %v", err)
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

	if err := az.configureAzureClients(); err != nil {
		return AzureManager{}, fmt.Errorf("failed to create azure manager: %v", err)
	}

	return az, nil
}

func (az *AzureManager) validate() error {
	cfg := az.Config

	if cfg.Cloud == "" {
		return fmt.Errorf("cloud is empty")
	}

	if cfg.Location == "" {
		return fmt.Errorf("location is empty")
	}

	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is empty")
	}

	if cfg.ResourceGroup == "" {
		return fmt.Errorf("resource group is empty")
	}

	if cfg.VnetName == "" {
		return fmt.Errorf("vnet name is empty")
	}

	if cfg.SubnetName == "" {
		return fmt.Errorf("subnet name is empty")
	}

	if cfg.LoadBalancerName == "" {
		return fmt.Errorf("load balancer name is empty")
	}

	return nil
}

func (az *AzureManager) configureAzureClients() error {
	env, err := auth.ParseAzureEnvironment(az.Cloud, az.ResourceManagerEndpoint, az.IdentitySystem)
	if err != nil {
		return err
	}

	servicePrincipalToken, err := auth.GetServicePrincipalToken(&az.AzureAuthConfig, env, env.ServiceManagementEndpoint)
	if err != nil {
		return err
	}
	if servicePrincipalToken == nil {
		return fmt.Errorf("no credential is provided")
	}
	az.Environment = *env

	azClientConfig := &azclients.ClientConfig{
		CloudName:               az.Config.Cloud,
		Location:                az.Config.Location,
		SubscriptionID:          az.Config.SubscriptionID,
		ResourceManagerEndpoint: az.Environment.ResourceManagerEndpoint,
		Authorizer:              autorest.NewBearerAuthorizer(servicePrincipalToken),
		Backoff:                 &retry.Backoff{Steps: 1},
		DisableAzureStackCloud:  true, // TODO: disable azure stack cloud for now
		UserAgent:               az.Config.UserAgent,
	}

	az.LoadBalancerClient = loadbalancerclient.New(azClientConfig)
	az.SubnetsClient = subnetclient.New(azClientConfig)
	az.VmssClient = vmssclient.New(azClientConfig)

	return nil
}

func (az *AzureManager) GetLBFrontendIPConfigurationID(name string) *string {
	return to.StringPtr(fmt.Sprintf(LBFrontendIPConfigTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLBBackendAddressPoolID(name string) *string {
	return to.StringPtr(fmt.Sprintf(LBBackendPoolIDTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLBProbeID(name string) *string {
	return to.StringPtr(fmt.Sprintf(LBProbeIDTemplate, az.SubscriptionID, az.LoadBalancerResourceGroup, az.LoadBalancerName, name))
}

func (az *AzureManager) GetLB() (*network.LoadBalancer, error) {
	lb, rerr := az.LoadBalancerClient.Get(context.Background(), az.LoadBalancerResourceGroup, az.LoadBalancerName, "")
	if rerr != nil {
		return nil, rerr.Error()
	}
	return &lb, nil
}

func (az *AzureManager) CreateOrUpdateLB(lb network.LoadBalancer) error {
	if rerr := az.LoadBalancerClient.CreateOrUpdate(context.Background(), az.LoadBalancerResourceGroup, to.String(lb.Name), lb, to.String(lb.Etag)); rerr != nil {
		return rerr.Error()
	}
	return nil
}

func (az *AzureManager) ListVMSS() ([]compute.VirtualMachineScaleSet, error) {
	vmssList, rerr := az.VmssClient.List(context.Background(), az.ResourceGroup)
	if rerr != nil {
		return nil, rerr.Error()
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
	vmss, rerr := az.VmssClient.Get(context.Background(), resourceGroup, vmssName)
	if rerr != nil {
		return nil, rerr.Error()
	}
	return &vmss, nil
}

func (az *AzureManager) GetSubnet() (*network.Subnet, error) {
	subnet, rerr := az.SubnetsClient.Get(context.Background(), az.VnetResourceGroup, az.VnetName, az.SubnetName, "")
	if rerr != nil {
		return nil, rerr.Error()
	}
	return &subnet, nil
}
