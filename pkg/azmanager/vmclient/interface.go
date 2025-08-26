// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package vmclient

import (
	"context"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
)

// Interface is a client interface for managing Azure virtual machines
type Interface interface {
	// Get gets a virtual machine
	Get(ctx context.Context, resourceGroupName, vmName string, options *compute.VirtualMachinesClientGetOptions) (*compute.VirtualMachine, error)
	
	// List lists all the virtual machines in a resource group
	List(ctx context.Context, resourceGroupName string) ([]*compute.VirtualMachine, error)
	
	// CreateOrUpdate creates or updates a virtual machine
	CreateOrUpdate(ctx context.Context, resourceGroupName, vmName string, vm compute.VirtualMachine) (*compute.VirtualMachine, error)
	
	// Update updates a virtual machine
	Update(ctx context.Context, resourceGroupName, vmName string, vm compute.VirtualMachineUpdate) (*compute.VirtualMachine, error)
	
	// Delete deletes a virtual machine
	Delete(ctx context.Context, resourceGroupName, vmName string) error
}
