@description('The location where AKS will be deployed')
param location string

@description('Name of the AKS cluster')
param aksClusterName string

@description('Network plugin for AKS')
@allowed(['kubenet', 'azure', 'azure-podsubnet', 'overlay', 'cilium', 'calico'])
param networkPlugin string

@description('DNS server IP for AKS')
param dnsServerIP string

@description('Pod CIDR for AKS')
param podCIDR string

@description('Service CIDR for AKS')
param serviceCIDR string

@description('AKS control plane identity resource ID')
param aksIdentityId string

@description('AKS kubelet identity resource ID')
param aksKubeletIdentityId string

@description('AKS subnet resource ID')
param subnetAksId string

@description('Gateway subnet resource ID')
param subnetGatewayId string

@description('Pod subnet resource ID')
param subnetPodId string

@description('Name of the AKS node pool for gateway nodes')
param gwNodePoolName string

// Helper function to determine network profile
var networkProfiles = {
  kubenet: {
    networkPlugin: 'kubenet'
    podCidr: podCIDR
    maxPods: 250
  }
  azure: {
    networkPlugin: 'azure'
    maxPods: 30
  }
  'azure-podsubnet': {
    networkPlugin: 'azure'
    podSubnetId: subnetPodId
    maxPods: 30
  }
  overlay: {
    networkPlugin: 'azure'
    networkPluginMode: 'overlay'
    podCidr: podCIDR
    maxPods: 250
  }
  cilium: {
    networkPlugin: 'azure'
    networkPluginMode: 'overlay'
    networkDataplane: 'cilium'
    podCidr: podCIDR
    maxPods: 250
  }
  calico: {
    networkPlugin: 'azure'
    networkPolicy: 'calico'
    maxPods: 30
  }
}

var selectedProfile = networkProfiles[networkPlugin]

// AKS Cluster
resource aksCluster 'Microsoft.ContainerService/managedClusters@2025-04-01' = {
  name: aksClusterName
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${aksIdentityId}': {}
    }
  }
  properties: {
    dnsPrefix: aksClusterName
    enableRBAC: true
    agentPoolProfiles: [
      {
        name: 'nodepool1'
        count: 2
        osType: 'Linux'
        mode: 'System'
        vnetSubnetID: subnetAksId
        maxPods: selectedProfile.maxPods
      }
      {
        name: gwNodePoolName
        count: 2
        osType: 'Linux'
        mode: 'User'
        vnetSubnetID: subnetGatewayId
        maxPods: selectedProfile.maxPods
        nodeLabels: {
          'node.kubernetes.io/exclude-from-external-load-balancers': 'true'
          'kubeegressgateway.azure.com/mode': 'true'
        }
        nodeTaints: [
          'kubeegressgateway.azure.com/mode=true:NoSchedule'
        ]
      }
    ]
    identityProfile: {
      kubeletidentity: {
        resourceId: aksKubeletIdentityId
      }
    }
    servicePrincipalProfile: {
      clientId: 'msi'
    }
    networkProfile: union({
      serviceCidr: serviceCIDR
      dnsServiceIP: dnsServerIP
    }, contains(selectedProfile, 'networkPlugin') ? {
      networkPlugin: selectedProfile.networkPlugin
    } : {}, contains(selectedProfile, 'networkPluginMode') ? {
      networkPluginMode: selectedProfile.networkPluginMode
    } : {}, contains(selectedProfile, 'networkDataplane') ? {
      networkDataplane: selectedProfile.networkDataplane
    } : {}, contains(selectedProfile, 'networkPolicy') ? {
      networkPolicy: selectedProfile.networkPolicy
    } : {}, contains(selectedProfile, 'podCidr') ? {
      podCidr: selectedProfile.podCidr
    } : {})
  }
}

// Outputs
output clusterName string = aksCluster.name
output nodeResourceGroup string = aksCluster.properties.nodeResourceGroup
