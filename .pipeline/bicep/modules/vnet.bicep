@description('The location where the virtual network will be deployed')
param location string

@description('Name of the virtual network')
param vnetName string

// Virtual Network
resource vnet 'Microsoft.Network/virtualNetworks@2023-05-01' = {
  name: vnetName
  location: location
  properties: {
    addressSpace: {
      addressPrefixes: [
        '10.243.0.0/16'
      ]
    }
    subnets: [
      {
        name: 'gateway'
        properties: {
          addressPrefix: '10.243.0.0/23'
        }
      }
      {
        name: 'aks'
        properties: {
          addressPrefix: '10.243.4.0/22'
        }
      }
      {
        name: 'pod'
        properties: {
          addressPrefix: '10.243.8.0/22'
        }
      }
    ]
  }
}

// Outputs
output vnetId string = vnet.id
output subnetGatewayId string = vnet.properties.subnets[0].id
output subnetAksId string = vnet.properties.subnets[1].id
output subnetPodId string = vnet.properties.subnets[2].id
