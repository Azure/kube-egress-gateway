package azureclients

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient"
)

type AzureClientsFactory interface {
	// get load balancers client
	GetLoadBalancersClient() (*loadbalancerclient.LoadBalancersClient, error)

	// get virtual machine scale sets client
	GetVirtualMachineScaleSetsClient() (*vmssclient.VirtualMachineScaleSetsClient, error)
}

type azureClientsFactory struct {
	credentials    azcore.TokenCredential
	subscriptionID string
}

func NewAzureClientsFactoryWithClientSecret(subscriptionID, tenantID, aadClientID, aadClientSecret string) (AzureClientsFactory, error) {
	credentials, err := azidentity.NewClientSecretCredential(tenantID, aadClientID, aadClientSecret, nil)
	if err != nil {
		return nil, err
	}
	return &azureClientsFactory{
		credentials:    credentials,
		subscriptionID: subscriptionID,
	}, nil
}

func NewAzureClientsFactoryWithManagedIdentity(subscriptionID, managedIdentityID string) (AzureClientsFactory, error) {
	credentials, err := azidentity.NewManagedIdentityCredential(&azidentity.ManagedIdentityCredentialOptions{ID: azidentity.ClientID(managedIdentityID)})
	if err != nil {
		return nil, err
	}
	return &azureClientsFactory{
		credentials:    credentials,
		subscriptionID: subscriptionID,
	}, nil
}

func (factory *azureClientsFactory) GetLoadBalancersClient() (*loadbalancerclient.LoadBalancersClient, error) {
	client, err := loadbalancerclient.NewLoadBalancersClient(factory.subscriptionID, factory.credentials, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetVirtualMachineScaleSetsClient() (*vmssclient.VirtualMachineScaleSetsClient, error) {
	client, err := vmssclient.NewVirtualMachineScaleSetsClient(factory.subscriptionID, factory.credentials, nil)
	if err != nil {
		return nil, err
	}
	return client, nil
}
