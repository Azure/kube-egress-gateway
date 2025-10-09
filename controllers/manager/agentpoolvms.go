// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"github.com/google/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

var (
	namespaceAgentPool = uuid.Must(uuid.Parse("2c96e82c-842f-11f0-8ea5-6bee14278ecd"))
)

type agentPoolVMs struct {
	agentPoolName string
	client.StatusClient
	*azmanager.AzureManager
}

type gatewayIPConfig struct {
	primaryIP   string
	secondaryIP string
	subnetID    string
}

func NewAgentPoolVM(agentPoolName string, c client.StatusClient, manager *azmanager.AzureManager) *agentPoolVMs {
	return &agentPoolVMs{
		agentPoolName: agentPoolName,
		StatusClient:  c,
		AzureManager:  manager,
	}
}

func (a *agentPoolVMs) Reconcile(ctx context.Context, vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration, ipPrefixID string, wantIPConfig bool) ([]string, error) {
	backendLBPoolID := a.GetLBBackendAddressPoolID(a.GetUniqueID())

	secondaryIPs := make([]string, 0)

	nics, err := a.ListNetworkInterfaces(ctx, "" /* empty resource group, just use default */)
	if err != nil {
		return nil, err
	}

	gatewayNICs := make([]*network.Interface, 0, len(nics))
	for i := range nics {
		if nics[i] == nil {
			continue
		}
		if _, ok := nics[i].Tags[consts.AKSStaticGatewayNICTagKey]; !ok {
			continue
		}
		if val, ok := nics[i].Tags[consts.AKSNodepoolTagKey]; !ok || !strings.EqualFold(vmConfig.Spec.GatewayNodepoolName, to.Val(val)) {
			continue
		}
		gatewayNICs = append(gatewayNICs, nics[i])
	}

	for i := range gatewayNICs {
		ip, err := a.reconcileNIC(ctx, vmConfig, gatewayNICs[i], ipPrefixID, to.Val(backendLBPoolID), wantIPConfig)
		if err != nil {
			return nil, err
		}
		secondaryIPs = append(secondaryIPs, ip)
	}
	return secondaryIPs, nil
}

func (a *agentPoolVMs) GetUniqueID() string {
	return uuid.NewMD5(namespaceAgentPool, []byte(a.agentPoolName)).String()
}

func (r *agentPoolVMs) getGatewayIPConfig(nic *network.Interface, name string) gatewayIPConfig {
	result := gatewayIPConfig{}
	for _, ipConfig := range nic.Properties.IPConfigurations {
		if ipConfig == nil || ipConfig.Properties == nil {
			continue
		}
		if strings.EqualFold(to.Val(ipConfig.Name), name) {
			result.secondaryIP = to.Val(ipConfig.Properties.PrivateIPAddress)
		} else if to.Val(ipConfig.Properties.Primary) {
			result.primaryIP = to.Val(ipConfig.Properties.PrivateIPAddress)
			if ipConfig.Properties.Subnet != nil {
				result.subnetID = to.Val(ipConfig.Properties.Subnet.ID)
			}
		}
	}
	return result
}

func (r *agentPoolVMs) reconcileNIC(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
	nic *network.Interface,
	ipPrefixID string,
	lbBackendpoolID string,
	wantIPConfig bool,
) (string, error) {
	logger := log.FromContext(ctx).WithValues("nic", to.Val(nic.ID), "wantIPConfig", wantIPConfig, "ipPrefixID", ipPrefixID)
	ctx = log.IntoContext(ctx, logger)
	ipConfigName := managedSubresourceName(vmConfig)

	b, err := json.Marshal(nic)
	if err != nil {
		logger.Error(err, "failed to marshal nic")
	}
	logger.Info("reconciling NIC", "before", string(b))

	if nic.Properties == nil {
		return "", fmt.Errorf("nic(%s) has empty properties", to.Val(nic.ID))
	}

	forceUpdate := false
	// check ProvisioningState
	if to.Val(nic.Properties.ProvisioningState) != network.ProvisioningStateSucceeded {
		logger.Info(fmt.Sprintf("VM ProvisioningState %q", to.Val(nic.Properties.ProvisioningState)))
		if to.Val(nic.Properties.ProvisioningState) == network.ProvisioningStateFailed {
			forceUpdate = true
			logger.Info(fmt.Sprintf("Force update for unexpected NIC ProvisioningState:%q", to.Val(nic.Properties.ProvisioningState)))
		}
	}

	needUpdate := false

	// check primary IP & secondary IP
	ipCfg := r.getGatewayIPConfig(nic, ipConfigName)
	if !forceUpdate && wantIPConfig && (ipCfg.primaryIP == "" || ipCfg.secondaryIP == "") {
		forceUpdate = true
		logger.Info("Force update for missing primary IP and/or secondary IP", "primaryIP", ipCfg.primaryIP, "secondaryIP", ipCfg.secondaryIP)
	}

	if ipCfg.subnetID == "" {
		return "", fmt.Errorf("no subnetID found for NIC(%s)", to.Val(nic.ID))
	}

	// expected IPconfig
	expectedIPConfig := &network.InterfaceIPConfiguration{
		Name: to.Ptr(ipConfigName),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                 to.Ptr(false),
			PrivateIPAddressVersion: to.Ptr(network.IPVersionIPv4),
			Subnet: &network.Subnet{
				ID: to.Ptr(ipCfg.subnetID),
			},
		},
	}

	if ipPrefixID != "" && wantIPConfig {
		// todo should we check ip version?
		expectedPublicIP := &network.PublicIPAddress{
			Location: to.Ptr(r.Location()),
			SKU: &network.PublicIPAddressSKU{
				// todo these should really match the publicIPPrefix settings instead of hardcoded
				Name: to.Ptr(network.PublicIPAddressSKUNameStandard),
				Tier: to.Ptr(network.PublicIPAddressSKUTierRegional),
			},
			Properties: &network.PublicIPAddressPropertiesFormat{
				PublicIPAddressVersion: to.Ptr(network.IPVersionIPv4),
				PublicIPPrefix: &network.SubResource{
					ID: to.Ptr(ipPrefixID),
				},
				PublicIPAllocationMethod: to.Ptr(network.IPAllocationMethodStatic),
			},
		}
		pipName, err := r.buildPublicIPName(ipPrefixID, to.Val(nic.Name))
		if err != nil {
			return "", err
		}

		// todo how do we handle if the publicIPPrefix is out of IPs?
		pip, err := r.CreateOrUpdatePublicIP(ctx, "", pipName, *expectedPublicIP)
		if err != nil {
			return "", err
		}
		expectedIPConfig.Properties.PublicIPAddress = pip
	}

	found := false
	for i := 0; i < len(nic.Properties.IPConfigurations); i++ {
		ipConfig := nic.Properties.IPConfigurations[i]
		if ipConfig == nil || ipConfig.Properties == nil {
			continue
		}

		if strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) {
			if !wantIPConfig || differentNIC(ipConfig, expectedIPConfig) {
				// remove at i
				nic.Properties.IPConfigurations = append(nic.Properties.IPConfigurations[:i], nic.Properties.IPConfigurations[i+1:]...)
				needUpdate = true
				continue
			}

			found = true
			break
		}
	}

	if wantIPConfig && !found {
		nic.Properties.IPConfigurations = append(nic.Properties.IPConfigurations, expectedIPConfig)
		needUpdate = true
	}

	missingLB := true

	for i := range nic.Properties.IPConfigurations {
		ipConfig := nic.Properties.IPConfigurations[i]
		if ipConfig == nil || ipConfig.Properties == nil || !to.Val(ipConfig.Properties.Primary) {
			continue
		}

		for j := range ipConfig.Properties.LoadBalancerBackendAddressPools {
			pool := ipConfig.Properties.LoadBalancerBackendAddressPools[j]
			if pool == nil {
				continue
			}
			if strings.EqualFold(to.Val(pool.ID), lbBackendpoolID) {
				missingLB = false
			}
		}

		if missingLB {
			if ipConfig.Properties.LoadBalancerBackendAddressPools == nil {
				ipConfig.Properties.LoadBalancerBackendAddressPools = make([]*network.BackendAddressPool, 0)
			}

			ipConfig.Properties.LoadBalancerBackendAddressPools = append(ipConfig.Properties.LoadBalancerBackendAddressPools, &network.BackendAddressPool{ID: to.Ptr(lbBackendpoolID)})
		}
	}

	if needUpdate || forceUpdate || missingLB {
		b, _ = json.Marshal(nic)
		logger.Info("updating nic", "after", string(b))
		if !needUpdate && forceUpdate {
			logger.Info("nic update by forceUpdate")
		}
		nicID := to.Val(nic.ID)
		nic, err = r.CreateOrUpdateNetworkInterface(ctx, "", to.Val(nic.Name), to.Val(nic))
		if err != nil {
			return "", fmt.Errorf("failed to update nic(%s): %w", nicID, err)
		}
		ipCfg = r.getGatewayIPConfig(nic, ipConfigName)
	}

	// return earlier if it's deleting event
	if !wantIPConfig {
		return "", nil
	}

	vmprofile := egressgatewayv1alpha1.GatewayVMProfile{
		NodeName:    to.Val(nic.Name), // is this going to be fine?
		PrimaryIP:   ipCfg.primaryIP,
		SecondaryIP: ipCfg.secondaryIP,
	}
	if vmConfig.Status == nil {
		vmConfig.Status = &egressgatewayv1alpha1.GatewayVMConfigurationStatus{}
	}
	for i, profile := range vmConfig.Status.GatewayVMProfiles {
		if profile.NodeName == vmprofile.NodeName {
			if profile.PrimaryIP != ipCfg.primaryIP || profile.SecondaryIP != ipCfg.secondaryIP {
				vmConfig.Status.GatewayVMProfiles[i].PrimaryIP = ipCfg.primaryIP
				vmConfig.Status.GatewayVMProfiles[i].SecondaryIP = ipCfg.secondaryIP
				logger.Info("GatewayVMConfiguration status updated", "primaryIP", ipCfg.primaryIP, "secondaryIP", ipCfg.secondaryIP)
				return ipCfg.secondaryIP, nil
			}
			logger.Info("GatewayVMConfiguration status not changed", "primaryIP", ipCfg.primaryIP, "secondaryIP", ipCfg.secondaryIP)
			return ipCfg.secondaryIP, nil
		}
	}

	logger.Info("GatewayVMConfiguration status updated for new nodes", "nodeName", vmprofile.NodeName, "primaryIP", ipCfg.primaryIP, "secondaryIP", ipCfg.secondaryIP)
	vmConfig.Status.GatewayVMProfiles = append(vmConfig.Status.GatewayVMProfiles, vmprofile)

	return ipCfg.secondaryIP, nil
}

func (a *agentPoolVMs) buildPublicIPName(id string, val string) (string, error) {
	ipPrefix, err := arm.ParseResourceID(id)
	if err != nil {
		return "", err
	}
	h := fnv.New64a()
	_, err = h.Write([]byte(val))
	if err != nil {
		return "", err
	}
	return ipPrefix.Name + fmt.Sprintf("-%x", h.Sum64()), nil
}

func differentNIC(a, b *network.InterfaceIPConfiguration) bool {
	if a.Properties == nil && b.Properties == nil {
		return false
	}
	if a.Properties == nil || b.Properties == nil {
		return true
	}
	prop1, prop2 := a.Properties, b.Properties
	if to.Val(prop1.Primary) != to.Val(prop2.Primary) ||
		to.Val(prop1.PrivateIPAddressVersion) != to.Val(prop2.PrivateIPAddressVersion) {
		return true
	}

	if (prop1.Subnet != nil) != (prop2.Subnet != nil) {
		return true
	} else if prop1.Subnet != nil && prop2.Subnet != nil && !strings.EqualFold(to.Val(prop1.Subnet.ID), to.Val(prop2.Subnet.ID)) {
		return true
	}

	pip1, pip2 := prop1.PublicIPAddress, prop2.PublicIPAddress
	if (pip1 == nil) != (pip2 == nil) {
		return true
	} else if pip1 != nil && pip2 != nil {
		if to.Val(pip1.Name) != to.Val(pip2.Name) {
			return true
		} else if (pip1.Properties != nil) != (pip2.Properties != nil) {
			return true
		} else if pip1.Properties != nil && pip2.Properties != nil {
			prefix1, prefix2 := pip1.Properties.PublicIPPrefix, pip2.Properties.PublicIPPrefix
			if (prefix1 != nil) != (prefix2 != nil) {
				return true
			} else if prefix1 != nil && prefix2 != nil && !strings.EqualFold(to.Val(prefix1.ID), to.Val(prefix2.ID)) {
				return true
			}
		}
	}
	return false
}
