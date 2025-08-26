// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package vmclient

import (
	"context"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
)

// Client implements the Interface
type Client struct {
	client *compute.VirtualMachinesClient
}

// New creates a new VM client
func New(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*Client, error) {
	client, err := compute.NewVirtualMachinesClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}

	return &Client{
		client: client,
	}, nil
}

// Get gets a virtual machine
func (c *Client) Get(ctx context.Context, resourceGroupName, vmName string, options *compute.VirtualMachinesClientGetOptions) (*compute.VirtualMachine, error) {
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	if vmName == "" {
		return nil, errors.New("parameter vmName cannot be empty")
	}

	resp, err := c.client.Get(ctx, resourceGroupName, vmName, options)
	if err != nil {
		return nil, err
	}
	return &resp.VirtualMachine, nil
}

// List lists all the virtual machines in a resource group
func (c *Client) List(ctx context.Context, resourceGroupName string) ([]*compute.VirtualMachine, error) {
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}

	pager := c.client.NewListPager(resourceGroupName, nil)
	var vms []*compute.VirtualMachine
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, v := range nextResult.Value {
			vm := v
			vms = append(vms, &vm)
		}
	}
	return vms, nil
}

// CreateOrUpdate creates or updates a virtual machine
func (c *Client) CreateOrUpdate(ctx context.Context, resourceGroupName, vmName string, vm compute.VirtualMachine) (*compute.VirtualMachine, error) {
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	if vmName == "" {
		return nil, errors.New("parameter vmName cannot be empty")
	}

	poller, err := c.client.BeginCreateOrUpdate(ctx, resourceGroupName, vmName, vm, nil)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualMachine, nil
}

// Update updates a virtual machine
func (c *Client) Update(ctx context.Context, resourceGroupName, vmName string, vm compute.VirtualMachineUpdate) (*compute.VirtualMachine, error) {
	if resourceGroupName == "" {
		return nil, errors.New("parameter resourceGroupName cannot be empty")
	}
	if vmName == "" {
		return nil, errors.New("parameter vmName cannot be empty")
	}

	poller, err := c.client.BeginUpdate(ctx, resourceGroupName, vmName, vm, nil)
	if err != nil {
		return nil, err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, err
	}

	return &resp.VirtualMachine, nil
}

// Delete deletes a virtual machine
func (c *Client) Delete(ctx context.Context, resourceGroupName, vmName string) error {
	if resourceGroupName == "" {
		return errors.New("parameter resourceGroupName cannot be empty")
	}
	if vmName == "" {
		return errors.New("parameter vmName cannot be empty")
	}

	poller, err := c.client.BeginDelete(ctx, resourceGroupName, vmName, nil)
	if err != nil {
		return err
	}

	_, err = poller.PollUntilDone(ctx, nil)
	return err
}
