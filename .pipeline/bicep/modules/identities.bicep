@description('The location where identities will be deployed')
param location string

@description('Name of the AKS control plane identity')
param aksIdentityName string

@description('Name of the AKS kubelet identity')
param aksKubeletIdentityName string

// AKS Control Plane Identity
resource aksIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: aksIdentityName
  location: location
}

// AKS Kubelet Identity
resource aksKubeletIdentity 'Microsoft.ManagedIdentity/userAssignedIdentities@2023-01-31' = {
  name: aksKubeletIdentityName
  location: location
}

// Outputs
output aksIdentityId string = aksIdentity.id
output aksKubeletIdentityId string = aksKubeletIdentity.id
output kubeletPrincipalId string = aksKubeletIdentity.properties.principalId
output kubeletClientId string = aksKubeletIdentity.properties.clientId
