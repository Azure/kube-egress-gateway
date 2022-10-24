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
