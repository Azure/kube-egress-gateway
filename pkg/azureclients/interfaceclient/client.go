package interfaceclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
)

type InterfacesClient struct {
	*network.InterfacesClient
}

func NewInterfacesClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*InterfacesClient, error) {
	client, err := network.NewInterfacesClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &InterfacesClient{client}, nil
}

func (client *InterfacesClient) GetVirtualMachineScaleSetNetworkInterface(ctx context.Context, resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, expand string) (*network.Interface, error) {
	var options *network.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions
	if expand != "" {
		options = &network.InterfacesClientGetVirtualMachineScaleSetNetworkInterfaceOptions{Expand: &expand}
	}
	resp, err := client.InterfacesClient.GetVirtualMachineScaleSetNetworkInterface(ctx, resourceGroupName, virtualMachineScaleSetName, virtualmachineIndex, networkInterfaceName, options)
	if err != nil {
		return nil, err
	}
	return &resp.Interface, nil
}
