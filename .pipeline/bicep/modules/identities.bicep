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

// Preflight needs the AKS identity to have the Managed Identity Operator role
// This allows the AKS cluster to manage the kubelet identity
resource roleAssignmentAksIdentity 'Microsoft.Authorization/roleAssignments@2022-04-01' = {
  name: guid(aksIdentity.id, 'Managed Identity Operator')
  scope: aksKubeletIdentity
  properties: {
    roleDefinitionId: subscriptionResourceId('Microsoft.Authorization/roleDefinitions', 'f1a07417-d97a-45cb-824c-7a7467783830') // Managed Identity Operator
    principalId: aksIdentity.properties.principalId
    principalType: 'ServicePrincipal'
  }
}

// Outputs
output aksIdentityId string = aksIdentity.id
output aksKubeletIdentityId string = aksKubeletIdentity.id
output kubeletPrincipalId string = aksKubeletIdentity.properties.principalId
output kubeletClientId string = aksKubeletIdentity.properties.clientId
