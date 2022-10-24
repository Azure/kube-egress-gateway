package vmssvmclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
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
