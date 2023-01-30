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

package vmssclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/utils"
)

type VirtualMachineScaleSetsClient struct {
	*compute.VirtualMachineScaleSetsClient
}

func NewVirtualMachineScaleSetsClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*VirtualMachineScaleSetsClient, error) {
	client, err := compute.NewVirtualMachineScaleSetsClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &VirtualMachineScaleSetsClient{client}, nil
}

func (client *VirtualMachineScaleSetsClient) Get(ctx context.Context, resourceGroupName string, vmScaleSetName string, expand string) (*compute.VirtualMachineScaleSet, error) {
	var options *compute.VirtualMachineScaleSetsClientGetOptions
	if expand != "" {
		exp := compute.ExpandTypesForGetVMScaleSets(expand)
		options = &compute.VirtualMachineScaleSetsClientGetOptions{Expand: &exp}
	}
	resp, err := client.VirtualMachineScaleSetsClient.Get(ctx, resourceGroupName, vmScaleSetName, options)
	if err != nil {
		return nil, err
	}
	return &resp.VirtualMachineScaleSet, nil
}

func (client *VirtualMachineScaleSetsClient) List(ctx context.Context, resourceGroupName string) ([]*compute.VirtualMachineScaleSet, error) {
	var vmssList []*compute.VirtualMachineScaleSet
	pager := client.VirtualMachineScaleSetsClient.NewListPager(resourceGroupName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		vmssList = append(vmssList, page.Value...)
	}
	return vmssList, nil
}

func (client *VirtualMachineScaleSetsClient) CreateOrUpdate(ctx context.Context, resourceGroupName, vmScaleSetName string, vmScaleSet compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
	resp, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[compute.VirtualMachineScaleSetsClientCreateOrUpdateResponse], error) {
		return client.VirtualMachineScaleSetsClient.BeginCreateOrUpdate(ctx, resourceGroupName, vmScaleSetName, vmScaleSet, nil)
	})
	if err != nil {
		return nil, err
	}
	return &resp.VirtualMachineScaleSet, nil
}
