// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package manager

import (
	"testing"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"

	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

func TestDifferentNIC(t *testing.T) {
	tests := []struct {
		name     string
		a        *network.InterfaceIPConfiguration
		b        *network.InterfaceIPConfiguration
		expected bool
	}{
		{
			name: "both nil properties",
			a: &network.InterfaceIPConfiguration{
				Properties: nil,
			},
			b: &network.InterfaceIPConfiguration{
				Properties: nil,
			},
			expected: false,
		},
		{
			name: "one nil properties",
			a: &network.InterfaceIPConfiguration{
				Properties: nil,
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{},
			},
			expected: true,
		},
		{
			name: "different primary values",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary: to.Ptr(true),
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary: to.Ptr(false),
				},
			},
			expected: true,
		},
		{
			name: "different IP versions",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv6),
				},
			},
			expected: true,
		},
		{
			name: "one nil subnet",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: nil,
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: &network.Subnet{ID: to.Ptr("subnet-1")},
				},
			},
			expected: true,
		},
		{
			name: "different subnet IDs",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: &network.Subnet{ID: to.Ptr("subnet-1")},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: &network.Subnet{ID: to.Ptr("subnet-2")},
				},
			},
			expected: true,
		},
		{
			name: "same subnet IDs case insensitive",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: &network.Subnet{ID: to.Ptr("SUBNET-1")},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Subnet: &network.Subnet{ID: to.Ptr("subnet-1")},
				},
			},
			expected: false,
		},
		{
			name: "one nil public IP",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: nil,
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{Name: to.Ptr("pip-1")},
				},
			},
			expected: true,
		},
		{
			name: "different public IP names",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{Name: to.Ptr("pip-1")},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{Name: to.Ptr("pip-2")},
				},
			},
			expected: true,
		},
		{
			name: "one nil public IP properties",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name:       to.Ptr("pip-1"),
						Properties: nil,
					},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name:       to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{},
					},
				},
			},
			expected: true,
		},
		{
			name: "one nil public IP prefix",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: nil,
						},
					},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-1")},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "different public IP prefixes",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-1")},
						},
					},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-2")},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "same public IP prefixes case insensitive",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("PREFIX-1")},
						},
					},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-1")},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "identical configurations",
			a: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary:                 to.Ptr(true),
					PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
					Subnet:                  &network.Subnet{ID: to.Ptr("subnet-1")},
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-1")},
						},
					},
				},
			},
			b: &network.InterfaceIPConfiguration{
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					Primary:                 to.Ptr(true),
					PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
					Subnet:                  &network.Subnet{ID: to.Ptr("subnet-1")},
					PublicIPAddress: &network.PublicIPAddress{
						Name: to.Ptr("pip-1"),
						Properties: &network.PublicIPAddressPropertiesFormat{
							PublicIPPrefix: &network.SubResource{ID: to.Ptr("prefix-1")},
						},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := differentNIC(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("differentNIC() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestBuildPublicIPName(t *testing.T) {
	tests := []struct {
		name        string
		ipPrefixID  string
		nicName     string
		expected    string
		expectError bool
	}{
		{
			name:       "valid resource ID and NIC name",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "aks-nodepool1-12345678-vmss000000",
			expected:   "test-prefix-65d70f8df610e8df", // Expected hash for this input
		},
		{
			name:       "different NIC name generates different hash",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "aks-nodepool1-12345678-vmss000001",
			expected:   "test-prefix-65d70e8df610e72c", // Different hash for different input
		},
		{
			name:       "same inputs generate same hash (deterministic)",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "aks-nodepool1-12345678-vmss000000",
			expected:   "test-prefix-65d70f8df610e8df", // Same as first test
		},
		{
			name:       "different prefix name with same NIC",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/another-prefix",
			nicName:    "aks-nodepool1-12345678-vmss000000",
			expected:   "another-prefix-65d70f8df610e8df", // Same hash, different prefix
		},
		{
			name:        "invalid resource ID format",
			ipPrefixID:  "invalid-resource-id",
			nicName:     "aks-nodepool1-12345678-vmss000000",
			expectError: true,
		},
		{
			name:        "empty resource ID",
			ipPrefixID:  "",
			nicName:     "aks-nodepool1-12345678-vmss000000",
			expectError: true,
		},
		{
			name:       "empty NIC name",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "",
			expected:   "test-prefix-cbf29ce484222325", // Hash of empty string
		},
		{
			name:       "long NIC name",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "very-long-nic-name-that-could-be-generated-in-some-scenarios-with-many-characters",
			expected:   "test-prefix-466e8d0aab442712", // Hash handles long strings
		},
		{
			name:       "NIC name with special characters",
			ipPrefixID: "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix",
			nicName:    "aks-nodepool1_test-vm.example",
			expected:   "test-prefix-cfafa91a0f6d073b", // Hash handles special characters
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &agentPoolVMs{}
			result, err := a.buildPublicIPName(tt.ipPrefixID, tt.nicName)

			if tt.expectError {
				if err == nil {
					t.Errorf("buildPublicIPName() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("buildPublicIPName() unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("buildPublicIPName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestBuildPublicIPNameConsistency(t *testing.T) {
	a := &agentPoolVMs{}
	ipPrefixID := "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix"
	nicName := "aks-nodepool1-12345678-vmss000000"

	// Call the function multiple times to ensure it's deterministic
	results := make([]string, 5)
	for i := 0; i < 5; i++ {
		result, err := a.buildPublicIPName(ipPrefixID, nicName)
		if err != nil {
			t.Fatalf("buildPublicIPName() unexpected error on iteration %d: %v", i, err)
		}
		results[i] = result
	}

	// All results should be identical
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			t.Errorf("buildPublicIPName() not consistent: iteration %d = %v, iteration 0 = %v", i, results[i], results[0])
		}
	}
}

func TestBuildPublicIPNameHashCollision(t *testing.T) {
	a := &agentPoolVMs{}
	ipPrefixID := "/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/rg-test/providers/Microsoft.Network/publicIPPrefixes/test-prefix"

	// Test different inputs to ensure they generate different hashes
	nicNames := []string{
		"aks-nodepool1-12345678-vmss000000",
		"aks-nodepool1-12345678-vmss000001",
		"aks-nodepool2-12345678-vmss000000",
		"different-nic-name",
	}

	results := make(map[string]string)

	for _, nicName := range nicNames {
		result, err := a.buildPublicIPName(ipPrefixID, nicName)
		if err != nil {
			t.Fatalf("buildPublicIPName() unexpected error for nicName %s: %v", nicName, err)
		}

		if existingNic, exists := results[result]; exists {
			t.Errorf("Hash collision detected: nicName %s and %s both generated %s", nicName, existingNic, result)
		}

		results[result] = nicName
	}
}
