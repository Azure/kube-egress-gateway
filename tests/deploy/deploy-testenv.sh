#!/bin/bash
set -e
RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}
LOCATION=${LOCATION:-"eastus"}
VNET_NAME=${VNET_NAME:-"gateway-vnet"}
AKS_ID_NAME=${AKS_ID_NAME:-"gateway-aks-id"}
AKS_CLUSTER_NAME=${AKS_CLUSTER_NAME:-"aks"}
NETWORK_PLUGIN=${NETWORK_PLUGIN:-"kubenet"}
DNS_SERVER_IP=${DNS_SERVER_IP:-"10.245.0.10"}
POD_CIDR=${POD_CIRD:-"10.244.0.0/16"}
SERVICE_CIDR=${SERVICE_CIDR:-"10.245.0.0/16"}
LB_NAME=${LB_NAME:-"gateway-ilb"}

: "${AZURE_SUBSCRIPTION_ID:?Environment variable empty or not defined.}"
: "${AZURE_TENANT_ID:?Environment variable empty or not defined.}"
: "${AZURE_CLIENT_ID:?Environment variable empty or not defined.}"
: "${AZURE_CLIENT_SECRET:?Environment variable empty or not defined.}"

# Create resource group
RG_EXISTS=$(az group exists -n ${RESOURCE_GROUP})
if [ "$RG_EXISTS" != "true" ]; then
    echo "Creating resource group: ${RESOURCE_GROUP}"
    az group create -n ${RESOURCE_GROUP} -l ${LOCATION}
else
    echo "Resource group ${RESOURCE_GROUP} already exists."
fi

# Create vnet and subnets
echo "Creating virtual network: ${VNET_NAME}"
VNET=$(az network vnet create -g ${RESOURCE_GROUP} -n ${VNET_NAME} --address-prefixes "10.243.0.0/16")
VNET_ID=$(echo ${VNET} | jq -r '. | .id')
SUBNET_GATEWAY=$(az network vnet subnet create -g ${RESOURCE_GROUP} --vnet-name ${VNET_NAME} -n "gateway" --address-prefixes "10.243.0.0/23")
SUBNET_AKS=$(az network vnet subnet create -g ${RESOURCE_GROUP} --vnet-name ${VNET_NAME} -n "aks" --address-prefixes "10.243.4.0/22")
SUBNET_POD=$(az network vnet subnet create -g ${RESOURCE_GROUP} --vnet-name ${VNET_NAME} -n "pod" --address-prefixes "10.243.8.0/22")

SUBNET_AKS_ID=$(echo ${SUBNET_AKS} | jq -r '. | .id')
SUBNET_POD_ID=$(echo ${SUBNET_POD} | jq -r '. | .id')
SUBNET_GATEWAY_ID=$(echo ${SUBNET_GATEWAY} | jq -r '. | .id')

# Create aks cluster
if [ "$NETWORK_PLUGIN" == "kubenet" ]; then
    NETWORK_PROFILE="--network-plugin kubenet --pod-cidr ${POD_CIDR}"
    NODEPOOL_PROFILE="--max-pods 250"
elif [ "$NETWORK_PLUGIN" == "azure" ]; then
    NETWORK_PROFILE="--network-plugin azure"
    NODEPOOL_PROFILE="--max-pods 30"
elif [ "$NETWORK_PLUGIN" == "azure-podsubnet" ]; then
    NETWORK_PROFILE="--network-plugin azure"
    NODEPOOL_PROFILE="--pod-subnet-id ${SUBNET_POD_ID}"
elif [ "$NETWORK_PLUGIN" == "overlay" ]; then
    NETWORK_PROFILE="--network-plugin azure --network-plugin-mode overlay --pod-cidr ${POD_CIDR}"
    NODEPOOL_PROFILE="--max-pods 250"
elif [ "$NETWORK_PLUGIN" == "cilium" ]; then
    NETWORK_PROFILE="--network-plugin azure --network-plugin-mode overlay --pod-cidr ${POD_CIDR} --enable-cilium-dataplane"
    NODEPOOL_PROFILE="--max-pods 250"
elif [ "$NETWORK_PLUGIN" == "calico" ]; then
    NETWORK_PROFILE="--network-plugin azure --network-policy calico"
    NODEPOOL_PROFILE="--max-pods 30"
else
    echo "Network plugin ${NETWORK_PLUGIN} is not supported, should be kubenet/azure/azure-podsubnet/overlay/cilium/calico"
fi

echo "Creating AKS cluster: ${AKS_CLUSTER_NAME}"
AKS=$(az aks create -n ${AKS_CLUSTER_NAME} -g ${RESOURCE_GROUP} -l ${LOCATION} --enable-managed-identity \
    --dns-name-prefix ${AKS_CLUSTER_NAME} --admin-username "azureuser" --generate-ssh-keys \
    --dns-service-ip ${DNS_SERVER_IP} --service-cidr ${SERVICE_CIDR} ${NETWORK_PROFILE} \
    --node-count 1 --vnet-subnet-id ${SUBNET_AKS_ID} ${NODEPOOL_PROFILE})

AKS_NODE_RESOURCE_GROUP=$(echo ${AKS} | jq -r '. | .nodeResourceGroup')

# Add gateway nodepool
echo "Adding gateway nodepool to AKS cluster"
GW_NODEPOOL=$(az aks nodepool add -g ${RESOURCE_GROUP} -n "gwnodepool" --cluster-name ${AKS_CLUSTER_NAME} --node-count 2 --vnet-subnet-id ${SUBNET_GATEWAY_ID} ${NODEPOOL_PROFILE} \
            --labels node.kubernetes.io/exclude-from-external-load-balancers=true todo.kubernetes.azure.com/mode=Gateway \
            --node-taints mode=gateway:NoSchedule)

readarray -t VMSS_INFO < <(az vmss list -g ${AKS_NODE_RESOURCE_GROUP} | jq -r '.[] | select(.name | contains("gwnodepool")) | .uniqueId,.name')
GW_VMSS_ID=${VMSS_INFO[0]}
GW_VMSS_NAME=${VMSS_INFO[1]}

# hack: add additional tag to GW VMSS
GW_VMSS=$(az vmss update --name ${GW_VMSS_NAME} -g ${AKS_NODE_RESOURCE_GROUP} --set tags.aks-managed-gatewayIPPrefixSize=31)

# Creating azure configuration file for the controllers
echo "Generating azure configuration file"
cat << EOF > ./azure.json
{
    "cloud": "AzurePublicCloud",
    "tenantId": "${AZURE_TENANT_ID}",
    "subscriptionId": "${AZURE_SUBSCRIPTION_ID}",
    "aadClientId": "${AZURE_CLIENT_ID}",
    "aadClientSecret": "${AZURE_CLIENT_SECRET}",
    "resourceGroup": "${AKS_NODE_RESOURCE_GROUP}",
    "location": "${LOCATION}",
    "loadBalancerName": "${LB_NAME}",
    "loadBalancerResourceGroup": "${RESOURCE_GROUP}",
    "vnetName": "${VNET_NAME}",
    "vnetResourceGroup": "${RESOURCE_GROUP}",
    "subnetName": "gateway"
}
EOF

echo "Test environment setup completed."