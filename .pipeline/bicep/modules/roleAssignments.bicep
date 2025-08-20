@description('Kubelet identity principal ID')
param kubeletPrincipalId string

@description('Node resource group name')
param nodeResourceGroupName string

// Network Contributor role for main resource group
module kubeletNetworkContributor 'roleAssignment.bicep' = {
  name: 'mc-rg-network-contributor'
  scope: resourceGroup()
  params: {
    principalId: kubeletPrincipalId
    roleDefinitionId: '4d97b98b-1d4f-4787-a291-c67834d212e7' // Network Contributor
    principalType: 'ServicePrincipal'
    roleName: 'NetworkContributor'
  }
}

// Network Contributor role for node resource group
module kubeletNetworkContributorNRG 'roleAssignment.bicep' = {
  name: 'node-rg-network-contributor'
  scope: resourceGroup(nodeResourceGroupName)
  params: {
    principalId: kubeletPrincipalId
    roleDefinitionId: '4d97b98b-1d4f-4787-a291-c67834d212e7' // Network Contributor
    principalType: 'ServicePrincipal'
    roleName: 'NetworkContributor'
  }
}

// todo: scope should only be the gateway vmss instead of the whole resource group
module kubeletVMContributorRole 'roleAssignment.bicep' = {
  name: 'kubelet-vm-contributor'
  scope: resourceGroup(nodeResourceGroupName)
  params: {
    principalId: kubeletPrincipalId
    roleDefinitionId: '9980e02c-c2be-4d73-94e8-173b1dc7cf3c' // Virtual Machine Contributor
    principalType: 'ServicePrincipal'
    roleName: 'VirtualMachineContributor'
  }
}
