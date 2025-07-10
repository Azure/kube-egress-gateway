#!/bin/bash
set -e

# Default values
RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}

: "${AZURE_SUBSCRIPTION_ID:?Environment variable empty or not defined.}"

echo "Cleaning up test environment resources"

# Check if resource group exists
RG_EXISTS=$(az group exists -n ${RESOURCE_GROUP})
if [ "$RG_EXISTS" == "true" ]; then
    echo "Deleting resource group: ${RESOURCE_GROUP}"
    az group delete -n ${RESOURCE_GROUP} --yes --no-wait
    echo "Resource group deletion initiated (running in background)"
else
    echo "Resource group ${RESOURCE_GROUP} does not exist"
fi

# Clean up generated files
if [ -f "azure.json" ]; then
    echo "Removing azure.json configuration file"
    rm -f azure.json
fi

echo "Cleanup completed"
