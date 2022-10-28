package vmssclient

import (
	"context"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

type Interface interface {
	// Get() gets a virtual machine scale set (vmss) object
	Get(ctx context.Context, resourceGroupName string, vmScaleSetName string, expand string) (*compute.VirtualMachineScaleSet, error)

	// List() gets a list of all VM scale sets
	List(ctx context.Context, resourceGrouName string) ([]*compute.VirtualMachineScaleSet, error)

	// CreateOrUpdate() creates or updates a virtual machine scale set (vmss) object
	CreateOrUpdate(ctx context.Context, resourceGroupName string, vmScaleSetName string, vmScaleSet compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error)
}
