#!/bin/bash
set -e

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BICEP_DIR="${SCRIPT_DIR}/../bicep"

# Default values
RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}
VNET_NAME=${VNET_NAME:-"gateway-vnet"}
AKS_ID_NAME=${AKS_ID_NAME:-"gateway-aks-id"}
AKS_KUBELET_ID_NAME=${AKS_KUBELET_ID_NAME:-"gateway-aks-kubelet-id"}
AKS_CLUSTER_NAME=${AKS_CLUSTER_NAME:-"aks"}
NETWORK_PLUGIN=${NETWORK_PLUGIN:-"overlay"}
DNS_SERVER_IP=${DNS_SERVER_IP:-"10.245.0.10"}
POD_CIDR=${POD_CIDR:-"10.244.0.0/16"}
SERVICE_CIDR=${SERVICE_CIDR:-"10.245.0.0/16"}
LB_NAME=${LB_NAME:-"kubeegressgateway-ilb"}

: "${AZURE_SUBSCRIPTION_ID:?Environment variable empty or not defined.}"
: "${AZURE_TENANT_ID:?Environment variable empty or not defined.}"

# Get random location
# should have quota for Standard_DS2_v2 vCPUs in AKS_UPSTREAM_E2E or custom sub
if [[ -z "${LOCATION}" ]]; then
    REGIONS=("eastus2" "northeurope" "westus2")
    LOCATION="${REGIONS[${RANDOM} % ${#REGIONS[@]}]}"
fi
echo "Deploying resources in region: ${LOCATION}"

# Create resource group
RG_EXISTS=$(az group exists -n ${RESOURCE_GROUP})
if [ "$RG_EXISTS" != "true" ]; then
    echo "Creating resource group: ${RESOURCE_GROUP}"
    az group create -n ${RESOURCE_GROUP} -l ${LOCATION} --tags usage=pod-egress-e2e creation_date="$(date)"
else
    echo "Resource group ${RESOURCE_GROUP} already exists."
fi

# Generate parameters file from template
echo "Generating Bicep parameters file"
TEMP_PARAMS_FILE=$(mktemp)
envsubst < "${BICEP_DIR}/parameters.json" > "${TEMP_PARAMS_FILE}"

# Deploy infrastructure using Bicep
echo "Deploying infrastructure using Bicep templates"
DEPLOYMENT_OUTPUT=$(az deployment group create \
    --resource-group ${RESOURCE_GROUP} \
    --template-file "${BICEP_DIR}/main.bicep" \
    --parameters "@${TEMP_PARAMS_FILE}" \
    --parameters location="${LOCATION}" \
    --output json)

# Clean up temporary parameters file
rm -f "${TEMP_PARAMS_FILE}"

# Extract outputs from deployment
echo "Extracting deployment outputs"
NODE_RESOURCE_GROUP=$(echo ${DEPLOYMENT_OUTPUT} | jq -r '.properties.outputs.nodeResourceGroup.value')
KUBELET_PRINCIPAL_ID=$(echo ${DEPLOYMENT_OUTPUT} | jq -r '.properties.outputs.kubeletPrincipalId.value')
KUBELET_CLIENT_ID=$(echo ${DEPLOYMENT_OUTPUT} | jq -r '.properties.outputs.kubeletClientId.value')
AZURE_CONFIG=$(echo ${DEPLOYMENT_OUTPUT} | jq '.properties.outputs.azureConfig.value')

# Post-deployment: Add VMSS tag and role assignment
echo "Performing post-deployment configurations"
readarray -t VMSS_INFO < <(az vmss list -g ${NODE_RESOURCE_GROUP} | jq -r '.[] | select(.name | contains("gwnodepool")) | .uniqueId,.name')
if [ ${#VMSS_INFO[@]} -ge 2 ]; then
    GW_VMSS_NAME=${VMSS_INFO[1]}
    
    # Add additional tag to GW VMSS
    echo "Adding tags to gateway VMSS: ${GW_VMSS_NAME}"
    az vmss update --name ${GW_VMSS_NAME} -g ${NODE_RESOURCE_GROUP} --set tags.aks-managed-gatewayIPPrefixSize=31 > /dev/null
    
    # Add Virtual Machine Contributor role for the specific VMSS
    echo "Assigning Virtual Machine Contributor role to kubelet identity for VMSS"
    az role assignment create --role "Virtual Machine Contributor" --assignee ${KUBELET_PRINCIPAL_ID} \
        --scope "/subscriptions/${AZURE_SUBSCRIPTION_ID}/resourceGroups/${NODE_RESOURCE_GROUP}/providers/Microsoft.Compute/virtualMachineScaleSets/${GW_VMSS_NAME}" > /dev/null
else
    echo "Warning: Gateway VMSS not found or not ready yet"
fi

# Generate azure configuration file for the controllers
echo "Generating azure configuration file: $(pwd)/azure.json"
echo ${AZURE_CONFIG} | jq '.' > $(pwd)/azure.json

if [[ ! -z "${KUBECONFIG_FILE}" ]]; then
  echo "Retrieving cluster kubeconfig"
  az aks get-credentials -g $RESOURCE_GROUP -n $AKS_CLUSTER_NAME --file ${KUBECONFIG_FILE} --overwrite-existing
  echo "Kubeconfig is saved at: $(pwd)/${KUBECONFIG_FILE}"
fi

echo "Test environment setup completed."
