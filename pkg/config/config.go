// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package config

import (
	"fmt"
	"strings"

	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/policy/ratelimit"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
)

type RateLimitConfig ratelimit.Config
type CloudConfig struct {
	azclient.ARMClientConfig `json:",inline" mapstructure:",squash"`
	azclient.AzureAuthConfig `json:",inline" mapstructure:",squash"`
	*RateLimitConfig         `json:",inline" mapstructure:",squash"`
	// azure resource location
	Location string `json:"location,omitempty" mapstructure:"location,omitempty"`
	// subscription ID
	SubscriptionID string `json:"subscriptionID,omitempty" mapstructure:"subscriptionID,omitempty"`
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
	// Enable exponential backoff to manage resource request retries
	CloudProviderBackoff bool `json:"cloudProviderBackoff,omitempty" mapstructure:"cloudProviderBackoff,omitempty"`
}

func (cfg *CloudConfig) DefaultAndValidate() error {
	cfg.trimSpace()

	if cfg.Cloud == "" {
		return fmt.Errorf("cloud is empty")
	}

	if cfg.Location == "" {
		return fmt.Errorf("location is empty")
	}

	if cfg.SubscriptionID == "" {
		return fmt.Errorf("subscription ID is empty")
	}

	if !cfg.UseManagedIdentityExtension {
		if cfg.UserAssignedIdentityID != "" {
			return fmt.Errorf("useManagedIdentityExtension needs to be true when userAssignedIdentityID is provided")
		}
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

	// default values
	if cfg.UserAgent == "" {
		cfg.UserAgent = consts.DefaultUserAgent
	}

	if cfg.LoadBalancerResourceGroup == "" {
		cfg.LoadBalancerResourceGroup = cfg.ResourceGroup
	}

	if cfg.VnetResourceGroup == "" {
		cfg.VnetResourceGroup = cfg.ResourceGroup
	}

	// if not specified, apply default rate limit config
	if cfg.RateLimitConfig == nil {
		cfg.RateLimitConfig = &RateLimitConfig{CloudProviderRateLimit: true}
	}

	if cfg.CloudProviderRateLimit {
		// Assign read rate limit defaults if no configuration was passed in.
		if cfg.CloudProviderRateLimitQPS == 0 {
			cfg.CloudProviderRateLimitQPS = consts.RateLimitQPSDefault
		}
		if cfg.CloudProviderRateLimitBucket == 0 {
			cfg.CloudProviderRateLimitBucket = consts.RateLimitBucketDefault
		}
		// Assign write rate limit defaults if no configuration was passed in.
		if cfg.CloudProviderRateLimitQPSWrite == 0 {
			cfg.CloudProviderRateLimitQPSWrite = cfg.CloudProviderRateLimitQPS
		}
		if cfg.CloudProviderRateLimitBucketWrite == 0 {
			cfg.CloudProviderRateLimitBucketWrite = cfg.CloudProviderRateLimitBucket
		}
	}
	return nil
}

func (cfg *CloudConfig) trimSpace() {
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
