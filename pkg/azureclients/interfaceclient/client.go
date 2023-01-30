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

package interfaceclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
)

type InterfacesClient struct {
	*network.InterfacesClient
}

func NewInterfacesClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*InterfacesClient, error) {
	client, err := network.NewInterfacesClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &InterfacesClient{client}, nil
}

func (client *InterfacesClient) GetVirtualMachineScaleSetNetworkInterface(ctx context.Context, resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, expand string) (*network.Interface, error) {
	var options *network.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions
	if expand != "" {
		options = &network.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions{Expand: &expand}
	}
	resp, err := client.InterfacesClient.GetVirtualMachineScaleSetNetworkInterface(ctx, resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, options)
	if err != nil {
		return nil, err
	}
	return &resp.Interface, nil
}
