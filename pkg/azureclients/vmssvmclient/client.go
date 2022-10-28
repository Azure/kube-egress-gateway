package vmssvmclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/utils"
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
