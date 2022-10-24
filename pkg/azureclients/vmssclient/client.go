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
	exp := compute.ExpandTypesForGetVMScaleSets(expand)
	resp, err := client.VirtualMachineScaleSetsClient.Get(ctx, resourceGroupName, vmScaleSetName, &compute.VirtualMachineScaleSetsClientGetOptions{Expand: &exp})
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

func (client *VirtualMachineScaleSetsClient) UpdateInstances(ctx context.Context, resourceGroupName string, vmScaleSetName string, vmInstanceIDs []*string) error {
	instanceIDs := compute.VirtualMachineScaleSetVMInstanceRequiredIDs{
		InstanceIDs: vmInstanceIDs,
	}
	_, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[compute.VirtualMachineScaleSetsClientUpdateInstancesResponse], error) {
		return client.VirtualMachineScaleSetsClient.BeginUpdateInstances(ctx, resourceGroupName, vmScaleSetName, instanceIDs, nil)
	})
	return err
}
