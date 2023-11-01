// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package config

import (
	"fmt"
	"strings"
)

type CloudConfig struct {
	// azure cloud
	Cloud string `json:"cloud,omitempty" mapstructure:"cloud,omitempty"`
	// azure resource location
	Location string `json:"location,omitempty" mapstructure:"location,omitempty"`
	// subscription ID
	SubscriptionID string `json:"subscriptionID,omitempty" mapstructure:"subscriptionID,omitempty"`
	// tenant ID
	TenantID string `json:"tenantID,omitempty" mapstructure:"tenantID,omitempty"`
	// use user assigned identity or not
	UseUserAssignedIdentity bool `json:"useUserAssignedIdentity,omitempty" mapstructure:"useUserAssignedIdentity,omitempty"`
	// user assigned identity ID
	UserAssignedIdentityID string `json:"userAssignedIdentityID,omitempty" mapstructure:"userAssignedIdentityID,omitempty"`
	// aad client ID
	AADClientID string `json:"aadClientID,omitempty" mapstructure:"aadClientID,omitempty"`
	// aad client secret
	AADClientSecret string `json:"aadClientSecret,omitempty" mapstructure:"aadClientSecret,omitempty"`
	// user agent for Azure customer usage attribution
	UserAgent string `json:"userAgent,omitempty" mapstructure:"userAgent,omitempty"`
	// default resource group where the gateway nodes are deployed
	ResourceGroup string `json:"resourceGroup,omitempty" mapstructure:"resourceGroup,omitempty"`
	// name of the gateway ILB
	LoadBalancerName string `json:"gatewayLoadBalancerName,omitempty" mapstructure:"gatewayLoadBalancerName,omitempty"`
	// resource group where the gateway ILB belongs
	LoadBalancerResourceGroup string `json:"loadBalancerResourceGroup,omitempty" mapstructure:"loadBalancerResourceGroup,omitempty"`
	// name of the virtual network where the gateway ILB is deployed
	VnetName string `json:"vnetName,omitempty" mapstructure:"vnetName,omitempty"`
	// name of the resource group where the virtual network is deployed
	VnetResourceGroup string `json:"vnetResourceGroup,omitempty" mapstructure:"vnetResourceGroup,omitempty"`
	// name of the subnet in the vnet where the gateway ILB is deployed
	SubnetName string `json:"subnetName,omitempty" mapstructure:"subnetName,omitempty"`
}

func (cfg *CloudConfig) TrimSpace() {
	cfg.Cloud = strings.TrimSpace(cfg.Cloud)
	cfg.Location = strings.TrimSpace(cfg.Location)
	cfg.SubscriptionID = strings.TrimSpace(cfg.SubscriptionID)
	cfg.TenantID = strings.TrimSpace(cfg.TenantID)
	cfg.UserAssignedIdentityID = strings.TrimSpace(cfg.UserAssignedIdentityID)
	cfg.AADClientID = strings.TrimSpace(cfg.AADClientID)
	cfg.AADClientSecret = strings.TrimSpace(cfg.AADClientSecret)
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)
	cfg.ResourceGroup = strings.TrimSpace(cfg.ResourceGroup)
	cfg.LoadBalancerName = strings.TrimSpace(cfg.LoadBalancerName)
	cfg.LoadBalancerResourceGroup = strings.TrimSpace(cfg.LoadBalancerResourceGroup)
	cfg.VnetName = strings.TrimSpace(cfg.VnetName)
	cfg.VnetResourceGroup = strings.TrimSpace(cfg.VnetResourceGroup)
	cfg.SubnetName = strings.TrimSpace(cfg.SubnetName)
}

func (cfg *CloudConfig) Validate() error {
	if cfg.Cloud == "" {
		return fmt.Errorf("cloud is empty")
	}

	if cfg.Location == "" {
		return fmt.Errorf("location is empty")
	}

	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is empty")
	}

	if cfg.UseUserAssignedIdentity {
		if cfg.UserAssignedIdentityID == "" {
			return fmt.Errorf("user assigned identity ID is empty")
		}
	} else {
		if cfg.AADClientID == "" || cfg.AADClientSecret == "" {
			return fmt.Errorf("AAD client ID or AAD client secret is empty")
		}
	}

	if cfg.ResourceGroup == "" {
		return fmt.Errorf("resource group is empty")
	}

	if cfg.VnetName == "" {
		return fmt.Errorf("virtual network name is empty")
	}

	if cfg.SubnetName == "" {
		return fmt.Errorf("virtual network subnet name is empty")
	}

	return nil
}
