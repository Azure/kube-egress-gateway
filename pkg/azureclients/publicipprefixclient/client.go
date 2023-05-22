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

package publicipprefixclient

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"

	"github.com/Azure/kube-egress-gateway/pkg/azureclients/utils"
)

type PublicIPPrefixesClient struct {
	*network.PublicIPPrefixesClient
}

func NewPublicIPPrefixesClient(subscriptionID string, credential azcore.TokenCredential, options *arm.ClientOptions) (*PublicIPPrefixesClient, error) {
	client, err := network.NewPublicIPPrefixesClient(subscriptionID, credential, options)
	if err != nil {
		return nil, err
	}
	return &PublicIPPrefixesClient{client}, nil
}

func (client *PublicIPPrefixesClient) Get(ctx context.Context, resourceGroupName, publicIPPrefixName string, expand string) (*network.PublicIPPrefix, error) {
	var options *network.PublicIPPrefixesClientGetOptions
	if expand != "" {
		options = &network.PublicIPPrefixesClientGetOptions{Expand: &expand}
	}
	resp, err := client.PublicIPPrefixesClient.Get(ctx, resourceGroupName, publicIPPrefixName, options)
	if err != nil {
		return nil, err
	}
	return &resp.PublicIPPrefix, nil
}

func (client *PublicIPPrefixesClient) List(ctx context.Context, resourceGroupName string) ([]*network.PublicIPPrefix, error) {
	var pipPrefixList []*network.PublicIPPrefix
	pager := client.PublicIPPrefixesClient.NewListPager(resourceGroupName, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		pipPrefixList = append(pipPrefixList, page.Value...)
	}
	return pipPrefixList, nil
}

func (client *PublicIPPrefixesClient) CreateOrUpdate(ctx context.Context, resourceGroupName, publicIPPrefixName string, publicIPPrefix network.PublicIPPrefix) (*network.PublicIPPrefix, error) {
	resp, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[network.PublicIPPrefixesClientCreateOrUpdateResponse], error) {
		return client.PublicIPPrefixesClient.BeginCreateOrUpdate(ctx, resourceGroupName, publicIPPrefixName, publicIPPrefix, nil)
	})
	if err != nil {
		return nil, err
	}
	return &resp.PublicIPPrefix, nil
}

func (client *PublicIPPrefixesClient) Delete(ctx context.Context, resourceGroupName, publicIPPrefixName string) error {
	_, err := utils.PollUntilDone(ctx, func() (*runtime.Poller[network.PublicIPPrefixesClientDeleteResponse], error) {
		return client.PublicIPPrefixesClient.BeginDelete(ctx, resourceGroupName, publicIPPrefixName, nil)
	})
	return err
}
