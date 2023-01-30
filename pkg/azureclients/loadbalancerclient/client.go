/*
   MIT License

   Copyright (c) Microsoft Corporation.

   Permission is hereby granted, free of charge, to any person obtaining a copy
   of this software and associated documentation files (the "Software"), to deal
   in the Software without restriction, including without limitation the rights
   to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
   copies of the Software, and to permit persons to whom the Software is
   furnished to do so, subject to the following conditions:

   The above copyright notice and this permission notice shall be included in all
   copies or substantial portions of the Software.

   THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
   IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
   FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
   AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
   LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
   OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
   SOFTWARE
*/

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

func (client *LoadBalancersClient) Delete(ctx context.Context, resourceGroupName, loadBalancerName string) error {
	_, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[network.LoadBalancersClientDeleteResponse], error) {
		return client.LoadBalancersClient.BeginDelete(ctx, resourceGroupName, loadBalancerName, nil)
	})
	return err
}
