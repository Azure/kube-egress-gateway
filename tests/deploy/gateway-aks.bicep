param location string
param clusterName string
param clusterId string
param networkPlugin string = 'kubenet'
param sku string = 'Standard_DS2_v2'
param adminUsername string = 'azureuser'
param aksSubnetId string
param gatewaySubnetId string
param podSubnetId string
param aksPodCidr string
param aksServiceCidr string
param dnsServiceIp string
param sshKeyData string
param minCount int
param maxCount int
param gatewayNodeCount int = 2
param gatewayNodepoolName string

resource aks 'Microsoft.ContainerService/managedClusters@2022-01-02-preview' = {
  name: clusterName
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${clusterId}': {}
    }
  }
  properties: {
    aadProfile: {
      managed: true
    }
    agentPoolProfiles: [
      {
        enableAutoScaling: true
        maxCount: maxCount
        // maxPods: networkPlugin == 'kubenet' ? 250 : 100 // AzureCNI v1
        count: minCount
        minCount: minCount
        name: 'nodepool1'
        vmSize: sku
        vnetSubnetID: aksSubnetId
        podSubnetID: networkPlugin == 'azure' ? podSubnetId : null // use AzureCNI v2
        mode: 'System'
      }
      {
        enableAutoScaling: false
        // maxPods: networkPlugin == 'kubenet' ? 250 : 15 // Min maxPods can be configured at the time
        count: gatewayNodeCount
        name: gatewayNodepoolName
        vmSize: sku
        vnetSubnetID: gatewaySubnetId
        podSubnetID: networkPlugin == 'azure' ? podSubnetId : null
        nodeLabels: {
          'node.kubernetes.io/exclude-from-external-load-balancers':'true'
          'todo.kubernetes.azure.com/mode': 'Gateway'
        }
        nodeTaints: [
          'mode=gateway:NoSchedule'
        ]
      }
    ]
    dnsPrefix: clusterName
    linuxProfile: {
      adminUsername: adminUsername
      ssh: {
        publicKeys: [
          {
            keyData: sshKeyData
          }
        ]
      }
    }
    networkProfile: {
      dnsServiceIP: dnsServiceIp
      networkPlugin: networkPlugin
      podCidr: networkPlugin == 'kubenet' ? aksPodCidr : null
      serviceCidr: aksServiceCidr
    }
    servicePrincipalProfile: {
      clientId: 'msi'
    }
  }
}

output nodeResourceGroup string = aks.properties.nodeResourceGroup
