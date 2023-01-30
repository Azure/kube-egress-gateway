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

package vmssvmclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/utils"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

type VirtualMachineScaleSetVMsClient struct {
	*compute.VirtualMachineScaleSetVMsClient
}

func NewVirtualMachineScaleSetVMsClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*VirtualMachineScaleSetVMsClient, error) {
	client, err := compute.NewVirtualMachineScaleSetVMsClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &VirtualMachineScaleSetVMsClient{client}, nil
}

func (client *VirtualMachineScaleSetVMsClient) Get(ctx context.Context, resourceGroupName, vmScaleSetName, instanceID, expand string) (*compute.VirtualMachineScaleSetVM, error) {
	var options *compute.VirtualMachineScaleSetVMsClientGetOptions
	if expand != "" {
		options = &compute.VirtualMachineScaleSetVMsClientGetOptions{Expand: to.Ptr(compute.InstanceViewTypes(expand))}
	}
	resp, err := client.VirtualMachineScaleSetVMsClient.Get(ctx, resourceGroupName, vmScaleSetName, instanceID, options)
	if err != nil {
		return nil, err
	}
	return &resp.VirtualMachineScaleSetVM, nil
}

func (client *VirtualMachineScaleSetVMsClient) List(ctx context.Context, resourceGroupName, vmScaleSetName string) ([]*compute.VirtualMachineScaleSetVM, error) {
	var vmList []*compute.VirtualMachineScaleSetVM
	pager := client.VirtualMachineScaleSetVMsClient.NewListPager(resourceGroupName, vmScaleSetName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		vmList = append(vmList, page.Value...)
	}
	return vmList, nil
}

func (client *VirtualMachineScaleSetVMsClient) Update(ctx context.Context, resourceGroupName, vmScaleSetName, instanceID string, vmssVM compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
	resp, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[compute.VirtualMachineScaleSetVMsClientUpdateResponse], error) {
		return client.VirtualMachineScaleSetVMsClient.BeginUpdate(ctx, resourceGroupName, vmScaleSetName, instanceID, vmssVM, nil)
	})
	if err != nil {
		return nil, err
	}
	return &resp.VirtualMachineScaleSetVM, nil
}
