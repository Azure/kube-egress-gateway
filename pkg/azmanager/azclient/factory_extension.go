// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package azclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"

	"github.com/Azure/kube-egress-gateway/pkg/azmanager/vmclient"
)

// ExtendedClientFactory extends the Azure client factory
type ExtendedClientFactory struct {
	azclient.ClientFactory
	vmClient vmclient.Interface
}

// ExtendClientFactory extends the Azure client factory with VM client support
func ExtendClientFactory(factory azclient.ClientFactory, subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*ExtendedClientFactory, error) {
	vmClient, err := vmclient.New(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}

	return &ExtendedClientFactory{
		ClientFactory: factory,
		vmClient:      vmClient,
	}, nil
}

// GetVirtualMachineClient gets a virtual machine client
func (f *ExtendedClientFactory) GetVirtualMachineClient() vmclient.Interface {
	return f.vmClient
}
