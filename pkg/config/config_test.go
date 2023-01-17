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
			UseUserAssignedIdentity: true,
			UserAssignedIdentityID:  "a",
			expectPass:              false,
		},
		"UserAssignedIdentityID empty": {
			Cloud:                   "c",
			Location:                "l",
			SubscriptionID:          "s",
			ResourceGroup:           "v",
			LoadBalancerName:        "",
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
				UseUserAssignedIdentity: test.UseUserAssignedIdentity,
				UserAssignedIdentityID:  test.UserAssignedIdentityID,
				AADClientID:             test.AADClientID,
				AADClientSecret:         test.AADClientSecret,
			}

			err := config.Validate()

			if test.expectPass && err != nil {
				t.Fatalf("failed to test Validate: expected pass: actual fail with err %s", err)
			}

			if !test.expectPass && err == nil {
				t.Fatal("failed to test Validate: expected fail: actual pass")
			}
		})
	}
}
