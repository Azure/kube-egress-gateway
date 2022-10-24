package publicipprefixclient

import (
	"context"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
)

type Interface interface {
	// Get() gets a public ip prefix object
	Get(ctx context.Context, resourceGroupName, publicIPPrefixName, expand string) (*network.PublicIPPrefix, error)

	// List() lists public ip prefixes in a resource group
	List(ctx context.Context, resourceGroupName string) ([]*network.PublicIPPrefix, error)

	// CreateOrUpdate() creates or updates a public ip prefix object
	CreateOrUpdate(ctx context.Context, resourceGroupName, publicIPPrefixName string, publicIPPrefix network.PublicIPPrefix) (*network.PublicIPPrefix, error)

	// Delete() deletes a public ip prefix object
	Delete(ctx context.Context, resourceGroupName, publicIPPrefixName string) error
}
