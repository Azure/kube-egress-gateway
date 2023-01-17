package config

import (
	"fmt"
	"strings"
)

type CloudConfig struct {
	// azure cloud
	Cloud string
	// azure resource location
	Location string
	// subscription ID
	SubscriptionID string
	// tenant ID
	TenantID string
	// use user assigned identity or not
	UseUserAssignedIdentity bool
	// user assigned identity ID
	UserAssignedIdentityID string
	// aad client ID
	AADClientID string
	// aad client secret
	AADClientSecret string
	// user agent for Azure customer usage attribution
	UserAgent string
	// default resource group where the gateway nodes are deployed
	ResourceGroup string
	// name of the gateway ILB
	LoadBalancerName string
	// resource group where the gateway ILB belongs
	LoadBalancerResourceGroup string
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

	if cfg.LoadBalancerName == "" {
		return fmt.Errorf("load balancer name is empty")
	}

	return nil
}
