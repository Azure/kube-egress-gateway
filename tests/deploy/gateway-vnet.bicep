param vnetName string
param location string
param vnetCidr string = '10.243.0.0/16'
param gatewaySubnet string = '10.243.0.0/23'
param aksSubnet string = '10.243.4.0/22'
param podSubnet string = '10.243.8.0/22'
param aksPrincipalId string

resource vnet 'Microsoft.Network/virtualNetworks@2021-05-01' = {
  name: vnetName
  location: location
  properties: {
    addressSpace: {
      addressPrefixes: [
        vnetCidr
      ]
    }
    subnets: [
      {
        name: 'gateway'
        properties: {
          addressPrefix: gatewaySubnet
        }
      }
      {
        name: 'aks'
        properties: {
          addressPrefix: aksSubnet
        }
      }
      {
        name: 'pod'
        properties: {
          addressPrefix: podSubnet
        }
      }
    ]
  }
}

var roleDefinitionId = '${subscription().id}/providers/Microsoft.Authorization/roleDefinitions/4d97b98b-1d4f-4787-a291-c67834d212e7'
resource userRbac 'Microsoft.Authorization/roleAssignments@2018-09-01-preview' = {
  name: guid(vnet.id, aksPrincipalId, roleDefinitionId)
  scope: vnet
  properties: {
    principalId: aksPrincipalId
    roleDefinitionId: roleDefinitionId
    principalType: 'ServicePrincipal'
  }
}

output gatewaySubnetId string = vnet.properties.subnets[0].id
output aksSubnetId string = vnet.properties.subnets[1].id
output podSubnetId string = vnet.properties.subnets[2].id
