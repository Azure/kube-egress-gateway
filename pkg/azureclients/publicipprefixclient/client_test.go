package publicipprefixclient

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func TestNewPublicIPPrefixesClient(t *testing.T) {
	tests := []struct {
		name           string
		subscriptionID string
		options        *arm.ClientOptions
	}{
		{
			name:           "TestNewPublicIPPrefixesClient",
			subscriptionID: "subID",
			options: &arm.ClientOptions{
				ClientOptions: azcore.ClientOptions{
					Cloud: cloud.AzurePublic,
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			credential, err := azidentity.NewDefaultAzureCredential(nil)
			if err != nil {
				t.Fatalf("NewDefaultAzureCredential() failed with error %v", err)
			}
			client, err := NewPublicIPPrefixesClient(test.subscriptionID, credential, test.options)
			if err != nil {
				t.Fatalf("NewPublicIPPrefixesClient() failed with error %v", err)
			}
			if client == nil {
				t.Fatal("NewPublicIPPrefixesClient() returns nil client")
			}
		})
	}
}
