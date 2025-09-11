#!/bin/bash
set -e

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BICEP_DIR="${SCRIPT_DIR}/../bicep"

# Default values
export RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}
export VNET_NAME=${VNET_NAME:-"gateway-vnet"}
export AKS_ID_NAME=${AKS_ID_NAME:-"gateway-aks-id"}
export AKS_KUBELET_ID_NAME=${AKS_KUBELET_ID_NAME:-"gateway-aks-kubelet-id"}
export AKS_CLUSTER_NAME=${AKS_CLUSTER_NAME:-"aks"}
export NETWORK_PLUGIN=${NETWORK_PLUGIN:-"overlay"}
export DNS_SERVER_IP=${DNS_SERVER_IP:-"10.245.0.10"}
export POD_CIDR=${POD_CIDR:-"10.244.0.0/16"}
export SERVICE_CIDR=${SERVICE_CIDR:-"10.245.0.0/16"}
export LB_NAME=${LB_NAME:-"kubeegressgateway-ilb"}
export GW_NODE_POOL_NAME=${GW_NODE_POOL_NAME:-"gwnodepool"}

export schema='$schema' # dumb hack to avoid wiping the value from parameters.json

# Create resource group
RG_EXISTS=$(az group exists -n ${RESOURCE_GROUP})
if [ "$RG_EXISTS" != "true" ]; then
    # Get random location
    # should have quota for Standard_DS2_v2 vCPUs in AKS_UPSTREAM_E2E or custom sub
    if [[ -z "${LOCATION}" ]]; then
        REGIONS=("eastus2" "northeurope" "westus2")
        LOCATION="${REGIONS[${RANDOM} % ${#REGIONS[@]}]}"
    fi
    echo "Deploying resources in region: ${LOCATION}"
    echo "Creating resource group: ${RESOURCE_GROUP}"
    az group create -n ${RESOURCE_GROUP} -l ${LOCATION} --tags usage=pod-egress-e2e creation_date="$(date)"
else
    echo "Resource group ${RESOURCE_GROUP} already exists."
fi

# Generate parameters file from template
echo "Generating Bicep parameters file"
TEMP_PARAMS_FILE=$(mktemp)
envsubst < "${BICEP_DIR}/parameters.json" > "${TEMP_PARAMS_FILE}"

echo "Using parameters:"
cat "${TEMP_PARAMS_FILE}"

# echo "performing what-if on deployment"
# az deployment group what-if --resource-group ${RESOURCE_GROUP} --template-file "${BICEP_DIR}/main.bicep" --parameters "@${TEMP_PARAMS_FILE}" --parameters location="${LOCATION}"

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

# Post-deployment: Add VMSS tag
echo "Performing post-deployment configurations"

# this has to be done manually because aks-managed tags are not allowed through aks-rp
VMSS=$(az vmss list -g ${NODE_RESOURCE_GROUP} | jq --arg GW_NODE_POOL_NAME "${GW_NODE_POOL_NAME}" -r '.[] | select(.tags["aks-managed-poolName"] == $GW_NODE_POOL_NAME) | .name')
az vmss update --name ${VMSS} -g ${NODE_RESOURCE_GROUP} --set tags.aks-managed-gatewayIPPrefixSize=31 > /dev/null

# Generate azure configuration file for the controllers
echo "Generating azure configuration file: $(pwd)/azure.json"
echo ${AZURE_CONFIG} | jq '.' > $(pwd)/azure.json

if [[ ! -z "${KUBECONFIG_FILE}" ]]; then
  echo "Retrieving cluster kubeconfig"
  az aks get-credentials -g $RESOURCE_GROUP -n $AKS_CLUSTER_NAME --file ${KUBECONFIG_FILE} --overwrite-existing
  echo "Kubeconfig is saved at: $(pwd)/${KUBECONFIG_FILE}"
fi

echo "Test environment setup completed."
