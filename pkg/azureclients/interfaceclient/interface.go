package interfaceclient

import (
	"context"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
)

type Interface interface {
	// GetVirtualMachineScaleSetNetworkInterface() gets a vmss interface object
	GetVirtualMachineScaleSetNetworkInterface(ctx context.Context, resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, expand string) (*network.Interface, error)
}
