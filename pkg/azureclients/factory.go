package azureclients

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient"
)

type AzureClientsFactory interface {
	// get load balancers client
	GetLoadBalancersClient() (*loadbalancerclient.LoadBalancersClient, error)

	// get virtual machine scale sets client
	GetVirtualMachineScaleSetsClient() (*vmssclient.VirtualMachineScaleSetsClient, error)

	// get virtual machine scale set vms client
	GetVirtualMachineScaleSetVMsClient() (*vmssvmclient.VirtualMachineScaleSetVMsClient, error)

	// get public ip prefixes client
	GetPublicIPPrefixesClient() (*publicipprefixclient.PublicIPPrefixesClient, error)
}

type azureClientsFactory struct {
	credentials    azcore.TokenCredential
	subscriptionID string
	clientOptions  *arm.ClientOptions
}

func NewAzureClientsFactoryWithClientSecret(cloud, subscriptionID, tenantID, aadClientID, aadClientSecret string) (AzureClientsFactory, error) {
	clientOptions, err := getClientOptions(cloud)
	if err != nil {
		return nil, err
	}
	credentials, err := azidentity.NewClientSecretCredential(tenantID, aadClientID, aadClientSecret, nil)
	if err != nil {
		return nil, err
	}
	return &azureClientsFactory{
		credentials:    credentials,
		subscriptionID: subscriptionID,
		clientOptions:  clientOptions,
	}, nil
}

func NewAzureClientsFactoryWithManagedIdentity(cloud, subscriptionID, managedIdentityID string) (AzureClientsFactory, error) {
	clientOptions, err := getClientOptions(cloud)
	if err != nil {
		return nil, err
	}
	credentials, err := azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ClientID(managedIdentityID)})
	if err != nil {
		return nil, err
	}
	return &azureClientsFactory{
		credentials:    credentials,
		subscriptionID: subscriptionID,
		clientOptions:  clientOptions,
	}, nil
}

func (factory *azureClientsFactory) GetLoadBalancersClient() (*loadbalancerclient.LoadBalancersClient, error) {
	client, err := loadbalancerclient.NewLoadBalancersClient(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetVirtualMachineScaleSetsClient() (*vmssclient.VirtualMachineScaleSetsClient, error) {
	client, err := vmssclient.NewVirtualMachineScaleSetsClient(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetVirtualMachineScaleSetVMsClient() (*vmssvmclient.VirtualMachineScaleSetVMsClient, error) {
	client, err := vmssvmclient.NewVirtualMachineScaleSetVMsClient(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetPublicIPPrefixesClient() (*publicipprefixclient.PublicIPPrefixesClient, error) {
	client, err := publicipprefixclient.NewPublicIPPrefixesClient(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func getClientOptions(azureCloud string) (*arm.ClientOptions, error) {
	var cloudConf cloud.Configuration
	switch azureCloud {
	case "AzurePublicCloud":
		cloudConf = cloud.AzurePublic
	case "AzureGovernment":
		cloudConf = cloud.AzureGovernment
	case "AzureChina":
		cloudConf = cloud.AzureChina
	default:
		return nil, fmt.Errorf("azure cloud(%s) is not suppported, supported: AzurePublicCloud, AzureGovernment, AzureChina", azureCloud)
	}
	return &arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloudConf,
		},
	}, nil
}
