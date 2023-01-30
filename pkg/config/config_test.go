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
package config

import (
	"testing"
)

func TestTrimSpace(t *testing.T) {
	t.Run("test spaces are trimmed", func(t *testing.T) {
		config := CloudConfig{
			Cloud:                     "  test  \n",
			Location:                  "  test  \n",
			SubscriptionID:            "  test  \n",
			TenantID:                  "  test  \t \n",
			UserAgent:                 "  test  \n",
			ResourceGroup:             "\r\n  test  \n",
			LoadBalancerName:          "  test  \r\n",
			LoadBalancerResourceGroup: "  test  \n",
			UseUserAssignedIdentity:   true,
			UserAssignedIdentityID:    "  test  \n",
			AADClientID:               "\n  test  \n",
			AADClientSecret:           "  test  \n",
			VnetName:                  "  test   ",
			VnetResourceGroup:         " \t  test   ",
			SubnetName:                "  test  ",
		}

		expected := CloudConfig{
			Cloud:                     "test",
			Location:                  "test",
			SubscriptionID:            "test",
			TenantID:                  "test",
			UserAgent:                 "test",
			ResourceGroup:             "test",
			LoadBalancerName:          "test",
			LoadBalancerResourceGroup: "test",
			UseUserAssignedIdentity:   true,
			UserAssignedIdentityID:    "test",
			AADClientID:               "test",
			AADClientSecret:           "test",
			VnetName:                  "test",
			VnetResourceGroup:         "test",
			SubnetName:                "test",
		}
		config.TrimSpace()
		if config != expected {
			t.Fatalf("failed to test TrimSpace: expect config fields are trimmed, got: %v", config)
		}
	})
}

func TestValidate(t *testing.T) {
	tests := map[string]struct {
		Cloud                   string
		Location                string
		SubscriptionID          string
		ResourceGroup           string
		LoadBalancerName        string
		VnetName                string
		SubnetName              string
		UseUserAssignedIdentity bool
		UserAssignedIdentityID  string
		AADClientID             string
		AADClientSecret         string
		expectPass              bool
	}{
		"Cloud empty": {
			Cloud:                   "",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"Location empty": {
			Cloud:                   "c",
			Location:                "",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"SubscriptionID empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"ResourceGroup empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"LoadBalancerName empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"VnetName empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"SubnetName empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"UserAssignedIdentityID empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "",
			expectPass:              false,
		},
		"AADClientID empty": {
			Cloud:            "c",
			Location:         "l",
			SubscriptionID:   "s",
			ResourceGroup:    "v",
			LoadBalancerName: "l",
			VnetName:         "v",
			SubnetName:       "s",
			AADClientID:      "",
			AADClientSecret:  "2",
			expectPass:       false,
		},
		"AADClientSEcret empty": {
			Cloud:            "c",
			Location:         "l",
			SubscriptionID:   "s",
			ResourceGroup:    "v",
			LoadBalancerName: "l",
			VnetName:         "v",
			SubnetName:       "s",
			AADClientID:      "1",
			AADClientSecret:  "",
			expectPass:       false,
		},
		"has all required properties with secret": {
			Cloud:            "c",
			Location:         "l",
			SubscriptionID:   "s",
			ResourceGroup:    "v",
			LoadBalancerName: "l",
			VnetName:         "v",
			SubnetName:       "s",
			AADClientID:      "1",
			AADClientSecret:  "2",
			expectPass:       true,
		},
		"has all required properties with msi": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "l",
			VnetName:                "v",
			SubnetName:              "s",
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "u",
			expectPass:              true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := CloudConfig{
				Cloud:                   test.Cloud,
				Location:                test.Location,
				SubscriptionID:          test.SubscriptionID,
				ResourceGroup:           test.ResourceGroup,
				LoadBalancerName:        test.LoadBalancerName,
				VnetName:                test.VnetName,
				SubnetName:              test.SubnetName,
				UseUserAssignedIdentity: test.UseUserAssignedIdentity,
				UserAssignedIdentityID:  test.UserAssignedIdentityID,
				AADClientID:             test.AADClientID,
				AADClientSecret:         test.AADClientSecret,
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
