package loadbalancerclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/utils"
)

type LoadBalancersClient struct {
	*network.LoadBalancersClient
}

func NewLoadBalancersClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*LoadBalancersClient, error) {
	client, err := network.NewLoadBalancersClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &LoadBalancersClient{client}, nil
}

func (client *LoadBalancersClient) Get(ctx context.Context, resourceGroupName string, loadBalancerName string, expand string) (*network.LoadBalancer, error) {
	var options *network.LoadBalancersClientGetOptions
	if expand != "" {
		options = &network.LoadBalancersClientGetOptions{Expand: &expand}
	}
	resp, err := client.LoadBalancersClient.Get(ctx, resourceGroupName, loadBalancerName, options)
	if err != nil {
		return nil, err
	}
	return &resp.LoadBalancer, nil
}

func (client *LoadBalancersClient) CreateOrUpdate(ctx context.Context, resourceGroupName, loadBalancerName string, loadBalancer network.LoadBalancer) (*network.LoadBalancer, error) {
	resp, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[network.LoadBalancersClientCreateOrUpdateResponse], error) {
		return client.LoadBalancersClient.BeginCreateOrUpdate(ctx, resourceGroupName, loadBalancerName, loadBalancer, nil)
	})
	if err != nil {
		return nil, err
	}
	return &resp.LoadBalancer, nil
}
