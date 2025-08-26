// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"fmt"
	"strings"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	"sigs.k8s.io/controller-runtime/pkg/log"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

// getGatewayVM gets the Azure VM for a VM-based gateway nodepool
func (r *GatewayVMConfigurationReconciler) getGatewayVM(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) (*compute.VirtualMachine, int32, error) {
	log := log.FromContext(ctx)
	// Check for the new GatewayPoolProfile first
	if vmConfig.Spec.GatewayPoolProfile.Type == "vm" {
		// GatewayPoolProfile with VM type
		log.Info(fmt.Sprintf("Getting VM with name %s in resource group %s from pool profile", 
			vmConfig.Spec.GatewayPoolProfile.Name, vmConfig.Spec.GatewayPoolProfile.ResourceGroup))
		vm, err := r.GetVM(ctx, vmConfig.Spec.GatewayPoolProfile.ResourceGroup, vmConfig.Spec.GatewayPoolProfile.Name)
		if err != nil {
			log.Error(err, "Failed to get VM from pool profile")
			return nil, 0, err
		}
		return vm, vmConfig.Spec.GatewayPoolProfile.PublicIpPrefixSize, nil
	// Check for VM-based nodepools
	} else if vmConfig.Spec.VmName != "" && vmConfig.Spec.VmResourceGroup != "" {
		// Explicit VM configuration (in the case gatewayVMProfile
		log.Info(fmt.Sprintf("Getting VM with name %s in resource group %s", vmConfig.Spec.VmName, vmConfig.Spec.VmResourceGroup))
		vm, err := r.GetVM(ctx, vmConfig.Spec.VmResourceGroup, vmConfig.Spec.VmName)
		if err != nil {
			log.Error(err, "Failed to get VM")
			return nil, 0, err
		}
		return vm, vmConfig.Spec.PublicIpPrefixSize, nil
	}
	
	return nil, 0, fmt.Errorf("VM-based gateway configuration not found")
}

// reconcileVM configures a VM as an egress gateway
func (r *GatewayVMConfigurationReconciler) reconcileVM(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
	vm *compute.VirtualMachine,
	ipPrefixID string,
	enableEgress bool,
) ([]string, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling VM %s for egress gateway", to.Val(vm.Name)))

	// Get the load balancer backend pool ID
	var lbBackendpoolID *string
	if enableEgress {
		var err error
		if lbBackendpoolID, err = r.getLBBackendPoolID(ctx, vmConfig); err != nil {
			return nil, err
		}
	}

	// Initialize array to collect private IPs
	var privateIPs []string
	
	// Define the IP configuration name to use for egress
	ipConfigName := consts.GatewayIPConfigName
	
	// Process network interfaces on the VM
	// Since VMs don't have a network profile with interface configurations like VMSS,
	// we need to handle them differently
	if vm.Properties != nil && vm.Properties.NetworkProfile != nil && vm.Properties.NetworkProfile.NetworkInterfaces != nil {
		for _, nicRef := range vm.Properties.NetworkProfile.NetworkInterfaces {
			if nicRef.ID == nil {
				continue
			}
			
			// Extract the NIC name from the ID
			nicID := to.Val(nicRef.ID)
			nicName := nicID[strings.LastIndex(nicID, "/")+1:]
			
			// Get the resource group from the ID
			// Format: /subscriptions/{sub}/resourceGroups/{rg}/providers/Microsoft.Network/networkInterfaces/{name}
			parts := strings.Split(nicID, "/")
			var nicRG string
			for i, part := range parts {
				if strings.EqualFold(part, "resourceGroups") && i+1 < len(parts) {
					nicRG = parts[i+1]
					break
				}
			}
			
			if nicRG == "" {
				log.Error(nil, "Could not extract resource group from NIC ID", "nicID", nicID)
				continue
			}
			
			// Get the network interface
			nic, err := r.GetVMInterface(ctx, nicRG, to.Val(vm.Name), nicName)
			if err != nil {
				log.Error(err, "Failed to get network interface", "nicName", nicName)
				continue
			}
			
			// Check if this NIC should have the egress gateway IP configuration
			if nic.Properties == nil || nic.Properties.IPConfigurations == nil {
				continue
			}
			
			// Whether we need to add a new IP configuration or update an existing one
			var wantIPConfig bool
			if enableEgress {
				if vmConfig.Spec.SecondaryInterfaceName != "" {
					// Use a specific secondary interface
					wantIPConfig = strings.EqualFold(nicName, vmConfig.Spec.SecondaryInterfaceName)
				} else {
					// Use the first interface
					wantIPConfig = true
				}
			}
			
			// Configure the NIC
			needUpdate, err := r.reconcileVMNetworkInterface(ctx, ipConfigName, ipPrefixID, to.Val(lbBackendpoolID), wantIPConfig, nic)
			if err != nil {
				log.Error(err, "Failed to reconcile VM network interface", "nicName", nicName)
				continue
			}
			
			if needUpdate {
				_, err = r.InterfaceClient.CreateOrUpdate(ctx, nicRG, nicName, *nic)
				if err != nil {
					log.Error(err, "Failed to update network interface", "nicName", nicName)
					continue
				}
			}
			
			// Collect private IPs from this interface
			for _, ipConfig := range nic.Properties.IPConfigurations {
				if ipConfig.Name != nil && strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) &&
					ipConfig.Properties != nil && ipConfig.Properties.PrivateIPAddress != nil {
					privateIPs = append(privateIPs, to.Val(ipConfig.Properties.PrivateIPAddress))
				}
			}
		}
	}
	
	return privateIPs, nil
}

// reconcileVMNetworkInterface configures a VM network interface for egress gateway
func (r *GatewayVMConfigurationReconciler) reconcileVMNetworkInterface(
	ctx context.Context,
	ipConfigName string,
	ipPrefixID string,
	lbBackendpoolID string,
	wantIPConfig bool,
	nic *network.Interface,
) (bool, error) {
	log := log.FromContext(ctx)
	
	if nic.Properties == nil || nic.Properties.IPConfigurations == nil {
		return false, fmt.Errorf("invalid network interface")
	}
	
	// Check if the egress IP config exists
	var existingIPConfig *network.InterfaceIPConfiguration
	for i := range nic.Properties.IPConfigurations {
		ipConfig := &nic.Properties.IPConfigurations[i]
		if ipConfig.Name != nil && strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) {
			existingIPConfig = ipConfig
			break
		}
	}
	
	// If egress is not wanted, but the config exists, remove it
	if !wantIPConfig && existingIPConfig != nil {
		log.Info("Removing egress IP configuration", "ipConfigName", ipConfigName)
		var newIPConfigs []*network.InterfaceIPConfiguration
		for i := range nic.Properties.IPConfigurations {
			ipConfig := &nic.Properties.IPConfigurations[i]
			if !strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) {
				newIPConfigs = append(newIPConfigs, ipConfig)
			}
		}
		nic.Properties.IPConfigurations = newIPConfigs
		return true, nil
	}
	
	// If egress is wanted but the config doesn't exist, create it
	if wantIPConfig && existingIPConfig == nil {
		log.Info("Adding egress IP configuration", "ipConfigName", ipConfigName)
		
		// Create a new IP configuration for the gateway
		newIPConfig := &network.InterfaceIPConfiguration{
			Name: to.Ptr(ipConfigName),
			Properties: &network.InterfaceIPConfigurationPropertiesFormat{
				Primary:                   to.Ptr(false),
				PrivateIPAllocationMethod: to.Ptr(network.IPAllocationMethodDynamic),
				LoadBalancerBackendAddressPools: []*network.BackendAddressPool{
					{
						ID: to.Ptr(lbBackendpoolID),
					},
				},
			},
		}
		
		// Add public IP prefix association if needed
		if ipPrefixID != "" {
			newIPConfig.Properties.PublicIPAddressConfiguration = &network.PublicIPAddressConfiguration{
				Name: to.Ptr(fmt.Sprintf("%s-pip", ipConfigName)),
				Properties: &network.PublicIPAddressConfigurationProperties{
					PublicIPPrefix: &network.SubResource{
						ID: to.Ptr(ipPrefixID),
					},
				},
			}
		}
		
		nic.Properties.IPConfigurations = append(nic.Properties.IPConfigurations, newIPConfig)
		return true, nil
	}
	
	// If the config exists and we want egress, check if it needs updates
	if wantIPConfig && existingIPConfig != nil {
		needUpdate := false
		
		// Update LB backend pool if needed
		var hasBackendPool bool
		if existingIPConfig.Properties != nil && existingIPConfig.Properties.LoadBalancerBackendAddressPools != nil {
			for _, pool := range existingIPConfig.Properties.LoadBalancerBackendAddressPools {
				if pool.ID != nil && strings.EqualFold(to.Val(pool.ID), lbBackendpoolID) {
					hasBackendPool = true
					break
				}
			}
		}
		
		if !hasBackendPool {
			log.Info("Adding backend pool to IP configuration", "ipConfigName", ipConfigName)
			if existingIPConfig.Properties == nil {
				existingIPConfig.Properties = &network.InterfaceIPConfigurationPropertiesFormat{}
			}
			if existingIPConfig.Properties.LoadBalancerBackendAddressPools == nil {
				existingIPConfig.Properties.LoadBalancerBackendAddressPools = []*network.BackendAddressPool{}
			}
			existingIPConfig.Properties.LoadBalancerBackendAddressPools = append(
				existingIPConfig.Properties.LoadBalancerBackendAddressPools,
				&network.BackendAddressPool{ID: to.Ptr(lbBackendpoolID)},
			)
			needUpdate = true
		}
		
		// Check public IP prefix if needed
		if ipPrefixID != "" {
			// Check if public IP prefix needs to be added or updated
			if existingIPConfig.Properties.PublicIPAddressConfiguration == nil {
				log.Info("Adding public IP prefix to IP configuration", "ipConfigName", ipConfigName)
				existingIPConfig.Properties.PublicIPAddressConfiguration = &network.PublicIPAddressConfiguration{
					Name: to.Ptr(fmt.Sprintf("%s-pip", ipConfigName)),
					Properties: &network.PublicIPAddressConfigurationProperties{
						PublicIPPrefix: &network.SubResource{
							ID: to.Ptr(ipPrefixID),
						},
					},
				}
				needUpdate = true
			} else if existingIPConfig.Properties.PublicIPAddressConfiguration.Properties == nil ||
				existingIPConfig.Properties.PublicIPAddressConfiguration.Properties.PublicIPPrefix == nil ||
				existingIPConfig.Properties.PublicIPAddressConfiguration.Properties.PublicIPPrefix.ID == nil ||
				!strings.EqualFold(to.Val(existingIPConfig.Properties.PublicIPAddressConfiguration.Properties.PublicIPPrefix.ID), ipPrefixID) {
				
				log.Info("Updating public IP prefix", "ipConfigName", ipConfigName)
				if existingIPConfig.Properties.PublicIPAddressConfiguration.Properties == nil {
					existingIPConfig.Properties.PublicIPAddressConfiguration.Properties = &network.PublicIPAddressConfigurationProperties{}
				}
				existingIPConfig.Properties.PublicIPAddressConfiguration.Properties.PublicIPPrefix = &network.SubResource{
					ID: to.Ptr(ipPrefixID),
				}
				needUpdate = true
			}
		} else if existingIPConfig.Properties.PublicIPAddressConfiguration != nil {
			// Remove public IP prefix if not needed
			log.Info("Removing public IP prefix from IP configuration", "ipConfigName", ipConfigName)
			existingIPConfig.Properties.PublicIPAddressConfiguration = nil
			needUpdate = true
		}
		
		return needUpdate, nil
	}
	
	return false, nil
}
