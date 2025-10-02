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
