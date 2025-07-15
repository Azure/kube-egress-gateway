#!/bin/bash
set -e

# Script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BICEP_DIR="${SCRIPT_DIR}/../bicep"

echo "Validating Bicep templates..."

# Validate main template
echo "Validating main.bicep..."
az bicep build --file "${BICEP_DIR}/main.bicep" --stdout > /dev/null
echo "✓ main.bicep is valid"

# Validate individual modules
echo "Validating modules..."
for module in "${BICEP_DIR}/modules"/*.bicep; do
    module_name=$(basename "$module")
    echo "Validating ${module_name}..."
    az bicep build --file "$module" --stdout > /dev/null
    echo "✓ ${module_name} is valid"
done

echo "All Bicep templates are valid!"

# Optional: Run what-if deployment if resource group exists
RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}
RG_EXISTS=$(az group exists -n ${RESOURCE_GROUP} 2>/dev/null || echo "false")

if [ "$RG_EXISTS" == "true" ] && [ ! -z "${AZURE_SUBSCRIPTION_ID:-}" ] && [ ! -z "${AZURE_TENANT_ID:-}" ]; then
    echo ""
    echo "Resource group exists. Would you like to run a what-if deployment? (y/N)"
    read -r response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        echo "Running what-if deployment..."
        
        # Set default values for what-if
        export VNET_NAME=${VNET_NAME:-"gateway-vnet"}
        export AKS_ID_NAME=${AKS_ID_NAME:-"gateway-aks-id"}
        export AKS_KUBELET_ID_NAME=${AKS_KUBELET_ID_NAME:-"gateway-aks-kubelet-id"}
        export AKS_CLUSTER_NAME=${AKS_CLUSTER_NAME:-"aks"}
        export NETWORK_PLUGIN=${NETWORK_PLUGIN:-"overlay"}
        export DNS_SERVER_IP=${DNS_SERVER_IP:-"10.245.0.10"}
        export POD_CIDR=${POD_CIDR:-"10.244.0.0/16"}
        export SERVICE_CIDR=${SERVICE_CIDR:-"10.245.0.0/16"}
        export LB_NAME=${LB_NAME:-"kubeegressgateway-ilb"}
        export LOCATION=${LOCATION:-"eastus2"}
        
        # Generate parameters file
        TEMP_PARAMS_FILE=$(mktemp)
        envsubst < "${BICEP_DIR}/parameters.json" > "${TEMP_PARAMS_FILE}"
        
        # Run what-if
        az deployment group what-if \
            --resource-group ${RESOURCE_GROUP} \
            --template-file "${BICEP_DIR}/main.bicep" \
            --parameters "@${TEMP_PARAMS_FILE}" \
            --parameters location="${LOCATION}"
        
        # Clean up
        rm -f "${TEMP_PARAMS_FILE}"
    fi
fi
