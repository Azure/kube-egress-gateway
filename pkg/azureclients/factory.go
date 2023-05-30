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

package azureclients

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/interfaceclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetvmclient"
)

type AzureClientsFactory interface {
	// get load balancers client
	GetLoadBalancersClient() (loadbalancerclient.Interface, error)

	// get virtual machine scale sets client
	GetVirtualMachineScaleSetsClient() (virtualmachinescalesetclient.Interface, error)

	// get virtual machine scale set vms client
	GetVirtualMachineScaleSetVMsClient() (virtualmachinescalesetvmclient.Interface, error)

	// get public ip prefixes client
	GetPublicIPPrefixesClient() (publicipprefixclient.Interface, error)

	// get interfaces client
	GetInterfacesClient() (interfaceclient.Interface, error)

	// get subnets client
	GetSubnetsClient() (subnetclient.Interface, error)
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

func (factory *azureClientsFactory) GetLoadBalancersClient() (loadbalancerclient.Interface, error) {
	client, err := loadbalancerclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetVirtualMachineScaleSetsClient() (virtualmachinescalesetclient.Interface, error) {
	client, err := virtualmachinescalesetclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetVirtualMachineScaleSetVMsClient() (virtualmachinescalesetvmclient.Interface, error) {
	client, err := virtualmachinescalesetvmclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetPublicIPPrefixesClient() (publicipprefixclient.Interface, error) {
	client, err := publicipprefixclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetInterfacesClient() (interfaceclient.Interface, error) {
	client, err := interfaceclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func (factory *azureClientsFactory) GetSubnetsClient() (subnetclient.Interface, error) {
	client, err := subnetclient.New(factory.subscriptionID, factory.credentials, factory.clientOptions)
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
