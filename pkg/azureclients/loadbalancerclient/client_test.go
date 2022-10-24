package loadbalancerclient

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func TestNewLoadBalancersClient(t *testing.T) {
	tests := []struct {
		name           string
		subscriptionID string
		options        *arm.ClientOptions
	}{
		{
			name:           "TestNewLoadBalancersClient",
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
			client, err := NewLoadBalancersClient(test.subscriptionID, credential, test.options)
			if err != nil {
				t.Fatalf("NewLoadBalancersClient() failed with error %v", err)
			}
			if client == nil {
				t.Fatal("NewLoadBalancersClient() returns nil client")
			}
		})
	}
}
