@description('The location where all resources will be deployed')
param location string = resourceGroup().location

@description('Name of the virtual network')
param vnetName string = 'gateway-vnet'

@description('Name of the AKS control plane identity')
param aksIdentityName string = 'gateway-aks-id'

@description('Name of the AKS kubelet identity')
param aksKubeletIdentityName string = 'gateway-aks-kubelet-id'

@description('Name of the AKS cluster')
param aksClusterName string = 'aks'

@description('Network plugin for AKS')
@allowed(['kubenet', 'azure', 'azure-podsubnet', 'overlay', 'cilium', 'calico'])
param networkPlugin string = 'overlay'

@description('DNS server IP for AKS')
param dnsServerIP string = '10.245.0.10'

@description('Pod CIDR for AKS')
param podCIDR string = '10.244.0.0/16'

@description('Service CIDR for AKS')
param serviceCIDR string = '10.245.0.0/16'

@description('Load balancer name')
param loadBalancerName string = 'kubeegressgateway-ilb'

@description('Azure subscription ID')
param subscriptionId string

@description('Azure tenant ID')
param tenantId string

@description('Gateway node pool name')
param gatewayNodePoolName string

// Virtual Network
module vnet 'modules/vnet.bicep' = {
  name: 'vnet-deployment'
  params: {
    location: location
    vnetName: vnetName
  }
}

// Managed Identities
module identities 'modules/identities.bicep' = {
  name: 'identities-deployment'
  params: {
    location: location
    aksIdentityName: aksIdentityName
    aksKubeletIdentityName: aksKubeletIdentityName
  }
}

// Role Assignments
module roleAssignments 'modules/roleAssignments.bicep' = {
  name: 'role-assignments-deployment'
  params: {
    kubeletPrincipalId: identities.outputs.kubeletPrincipalId
    nodeResourceGroupName: aks.outputs.nodeResourceGroup
  }
}

// AKS Cluster
module aks 'modules/aks.bicep' = {
  name: 'aks-deployment'
  params: {
    location: location
    aksClusterName: aksClusterName
    networkPlugin: networkPlugin
    dnsServerIP: dnsServerIP
    podCIDR: podCIDR
    serviceCIDR: serviceCIDR
    aksIdentityId: identities.outputs.aksIdentityId
    aksKubeletIdentityId: identities.outputs.aksKubeletIdentityId
    subnetAksId: vnet.outputs.subnetAksId
    subnetGatewayId: vnet.outputs.subnetGatewayId
    subnetPodId: vnet.outputs.subnetPodId
    gatewayNodePoolName: gatewayNodePoolName
  }
}

// Outputs
output vnetId string = vnet.outputs.vnetId
output subnetAksId string = vnet.outputs.subnetAksId
output subnetGatewayId string = vnet.outputs.subnetGatewayId
output subnetPodId string = vnet.outputs.subnetPodId
output aksIdentityId string = identities.outputs.aksIdentityId
output aksKubeletIdentityId string = identities.outputs.aksKubeletIdentityId
output kubeletPrincipalId string = identities.outputs.kubeletPrincipalId
output kubeletClientId string = identities.outputs.kubeletClientId
output aksClusterName string = aks.outputs.clusterName
output nodeResourceGroup string = aks.outputs.nodeResourceGroup
output azureConfig object = {
  cloud: 'AzurePublicCloud'
  tenantId: tenantId
  subscriptionId: subscriptionId
  useManagedIdentityExtension: true
  userAssignedIdentityID: identities.outputs.kubeletClientId
  resourceGroup: aks.outputs.nodeResourceGroup
  location: location
  gatewayLoadBalancerName: loadBalancerName
  loadBalancerResourceGroup: resourceGroup().name
  vnetName: vnetName
  vnetResourceGroup: resourceGroup().name
  subnetName: 'gateway'
}
