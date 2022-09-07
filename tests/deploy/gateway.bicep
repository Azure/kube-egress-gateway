param adminUsername string = 'azureuser'
param adminSSHKey string
param location string = resourceGroup().location
param vnetName string = 'vnet-static-gateway'
param aksName string = 'aks'
param aksNetworkPlugin string = 'azure'
param vnetCidr string = '10.243.0.0/16'
param gatewaySubnet string = '10.243.0.0/23'
param aksSubnet string = '10.243.4.0/22'
param podSubnet string = '10.243.8.0/22'
param aksPodCidr string = '10.244.0.0/16'
param aksServiceCidr string = '10.245.0.0/16'
param dnsServiceIp string = '10.245.0.10'
param gatewayNodepoolName string = 'gwnodepool'
param loadBalancerName string = '${gatewayNodepoolName}-ilb'

// user assigned managed identity for the AKS cluster to access the network
resource aksId 'Microsoft.ManagedIdentity/userAssignedIdentities@2018-11-30' = {
  name: 'gateway-aks-id'
  location: location
}

module vnet './gateway-vnet.bicep' = {
  name: 'gateway-vnet'
  params: {
    location: location
    vnetName: vnetName
    vnetCidr: vnetCidr
    gatewaySubnet: gatewaySubnet
    aksSubnet: aksSubnet
    podSubnet: podSubnet
    aksPrincipalId: aksId.properties.principalId
  }
}

module aks './gateway-aks.bicep' = {
  name: 'gateway-aks'
  params: {
    location: location
    clusterName: aksName
    clusterId: aksId.id
    aksSubnetId: vnet.outputs.aksSubnetId
    gatewaySubnetId: vnet.outputs.gatewaySubnetId
    podSubnetId: vnet.outputs.podSubnetId
    networkPlugin: aksNetworkPlugin
    aksPodCidr: aksPodCidr
    aksServiceCidr: aksServiceCidr
    dnsServiceIp: dnsServiceIp
    sku: 'Standard_DS2_v2'
    minCount: 1
    maxCount: 25
    adminUsername: adminUsername
    sshKeyData: adminSSHKey
    gatewayNodepoolName: gatewayNodepoolName
  }
}

module gatewayILB 'gateway-ilb.bicep' =  {
  name: 'gateway-ilb'
  params: {
    loadBalancerName: loadBalancerName
    location: location
    aksPrincipalId: aksId.properties.principalId
  }
}
