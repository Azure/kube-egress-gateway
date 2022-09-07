param location string = resourceGroup().location
param loadBalancerName string
param aksPrincipalId string

// Deploy an ILB to be used as the wireguard endpoint
resource ilb 'Microsoft.Network/loadBalancers@2020-05-01' = {
  name: loadBalancerName
  location: location
  sku: {
    name: 'Standard'
  }
}

var roleDefinitionId = '${subscription().id}/providers/Microsoft.Authorization/roleDefinitions/4d97b98b-1d4f-4787-a291-c67834d212e7'
resource userRbac 'Microsoft.Authorization/roleAssignments@2018-09-01-preview' = {
  name: guid(ilb.id, aksPrincipalId, roleDefinitionId)
  scope: ilb
  properties: {
    principalId: aksPrincipalId
    roleDefinitionId: roleDefinitionId
    principalType: 'ServicePrincipal'
  }
}
