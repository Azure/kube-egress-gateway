// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package config

import (
	"testing"

	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
)

func TestTrimSpace(t *testing.T) {
	t.Run("test spaces are trimmed", func(t *testing.T) {
		config := CloudConfig{
			ARMClientConfig: azclient.ARMClientConfig{
				Cloud:     "  test  \n",
				UserAgent: "  test  \n",
				TenantID:  "  test  \t \n",
			},
			AzureAuthConfig: azclient.AzureAuthConfig{
				UserAssignedIdentityID:      "  test  \n",
				UseManagedIdentityExtension: true,
				AADClientID:                 "\n  test  \n",
				AADClientSecret:             "  test  \n",
			},
			Location:                  "  test  \n",
			SubscriptionID:            "  test  \n",
			ResourceGroup:             "\r\n  test  \n",
			LoadBalancerName:          "  test  \r\n",
			LoadBalancerResourceGroup: "  test  \n",
			VnetName:                  "  test   ",
			VnetResourceGroup:         " \t  test   ",
			SubnetName:                "  test  ",
		}

		expected := CloudConfig{
			ARMClientConfig: azclient.ARMClientConfig{
				Cloud:     "test",
				TenantID:  "test",
				UserAgent: "test",
			},
			Location:                  "test",
			SubscriptionID:            "test",
			ResourceGroup:             "test",
			LoadBalancerName:          "test",
			LoadBalancerResourceGroup: "test",
			AzureAuthConfig: azclient.AzureAuthConfig{
				UseManagedIdentityExtension: true,
				UserAssignedIdentityID:      "test",
				AADClientID:                 "test",
				AADClientSecret:             "test",
			},
			VnetName:          "test",
			VnetResourceGroup: "test",
			SubnetName:        "test",
		}
		config.TrimSpace()
		if config != expected {
			t.Fatalf("failed to test TrimSpace: expect config fields are trimmed, got: %v", config)
		}
	})
}

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		Cloud                       string
		Location                    string
		SubscriptionID              string
		ResourceGroup               string
		VnetName                    string
		SubnetName                  string
		UseManagedIdentityExtension bool
		UserAssignedIdentityID      string
		AADClientID                 string
		AADClientSecret             string
		expectPass                  bool
	}{
		"Cloud empty": {
			Cloud:                       "",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"Location empty": {
			Cloud:                       "c",
			Location:                    "",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"SubscriptionID empty": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"ResourceGroup empty": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"VnetName empty": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"SubnetName empty": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "a",
			expectPass:                  false,
		},
		"UserAssignedIdentityID not empty when UseManagedIdentityExtension is false": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: false,
			UserAssignedIdentityID:      "aaaa",
			expectPass:                  false,
		},
		"AADClientID empty": {
			Cloud:           "c",
			Location:        "l",
			SubscriptionID:  "s",
			ResourceGroup:   "v",
			VnetName:        "v",
			SubnetName:      "s",
			AADClientID:     "",
			AADClientSecret: "2",
			expectPass:      false,
		},
		"AADClientSEcret empty": {
			Cloud:           "c",
			Location:        "l",
			SubscriptionID:  "s",
			ResourceGroup:   "v",
			VnetName:        "v",
			SubnetName:      "s",
			AADClientID:     "1",
			AADClientSecret: "",
			expectPass:      false,
		},
		"has all required properties with secret": {
			Cloud:           "c",
			Location:        "l",
			SubscriptionID:  "s",
			ResourceGroup:   "v",
			VnetName:        "v",
			SubnetName:      "s",
			AADClientID:     "1",
			AADClientSecret: "2",
			expectPass:      true,
		},
		"has all required properties with msi": {
			Cloud:                       "c",
			Location:                    "l",
			SubscriptionID:              "s",
			ResourceGroup:               "v",
			VnetName:                    "v",
			SubnetName:                  "s",
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID:      "u",
			expectPass:                  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := CloudConfig{
				ARMClientConfig: azclient.ARMClientConfig{
					Cloud: test.Cloud,
				},
				AzureAuthConfig: azclient.AzureAuthConfig{
					UseManagedIdentityExtension: test.UseManagedIdentityExtension,
					UserAssignedIdentityID:      test.UserAssignedIdentityID,
					AADClientID:                 test.AADClientID,
					AADClientSecret:             test.AADClientSecret,
				},
				Location:       test.Location,
				SubscriptionID: test.SubscriptionID,
				ResourceGroup:  test.ResourceGroup,
				VnetName:       test.VnetName,
				SubnetName:     test.SubnetName,
			}

			err := config.Validate()

			if test.expectPass && err != nil {
				t.Fatalf("failed to test Validate: expected pass: actual fail with err(%s)", err)
			}

			if !test.expectPass && err == nil {
				t.Fatal("failed to test Validate: expected fail: actual pass")
			}
		})
	}
}
