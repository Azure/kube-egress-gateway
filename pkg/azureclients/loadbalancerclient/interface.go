package loadbalancerclient

import (
	"context"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
)

type Interface interface {
	// Get() gets a load balancer object
	Get(ctx context.Context, resourceGroupName string, loadBalancerName string, expand string) (*network.LoadBalancer, error)

	// CreateOrUpdate() creates or updates a load balancer object
	CreateOrUpdate(ctx context.Context, resourceGroupName string, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error)
}
