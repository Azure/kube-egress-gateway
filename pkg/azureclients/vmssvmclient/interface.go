package vmssvmclient

import (
	"context"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

type Interface interface {
	// List() gets a list of VMs in VM scale sets
	List(ctx context.Context, resourceGrouName, vmScaleSetName string) ([]*compute.VirtualMachineScaleSetVM, error)
}
