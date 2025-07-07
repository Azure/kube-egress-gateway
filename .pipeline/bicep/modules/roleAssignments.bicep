@description('Azure subscription ID')
param subscriptionId string

@description('Kubelet identity principal ID')
param kubeletPrincipalId string

@description('Node resource group name')
param nodeResourceGroupName string

// Network Contributor role for main resource group
module mainResourceGroupNetworkRole 'roleAssignment.bicep' = {
  name: 'main-rg-network-role'
  scope: resourceGroup()
  params: {
    principalId: kubeletPrincipalId
    roleDefinitionId: '4d97b98b-1d4f-4787-a291-c67834d212e7' // Network Contributor
    principalType: 'ServicePrincipal'
    roleName: 'NetworkContributor'
  }
}

// Network Contributor role for node resource group
module nodeResourceGroupNetworkRole 'roleAssignment.bicep' = {
  name: 'node-rg-network-role'
  scope: resourceGroup(subscriptionId, nodeResourceGroupName)
  params: {
    principalId: kubeletPrincipalId
    roleDefinitionId: '4d97b98b-1d4f-4787-a291-c67834d212e7' // Network Contributor
    principalType: 'ServicePrincipal'
    roleName: 'NetworkContributor'
  }
}
