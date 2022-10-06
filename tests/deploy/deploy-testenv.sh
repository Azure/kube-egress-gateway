#!/bin/bash
set -e
RESOURCE_GROUP=${RESOURCE_GROUP:-"kube-egress-gw-rg"}
LOCATION=${LOCATION:-"eastus"}
VNET_NAME=${VNET_NAME:-"gateway-vnet"}
AKS_ID_NAME=${AKS_ID_NAME:-"gateway-aks-id"}
AKS_CLUSTER_NAME=${AKS_CLUSTER_NAME:-"aks"}
NETWORK_PLUGIN=${NETWORK_PLUGIN:-"kubenet"}
DNS_SERVER_IP=${DNS_SERVER_IP:-"10.245.0.10"}
KUBENET_POD_CIDR=${KUBENET_POD_CIRD:-"10.244.0.0/16"}
SERVICE_CIDR=${SERVICE_CIDR:-"10.245.0.0/16"}
LB_NAME=${LB_NAME:-"gateway-ilb"}

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

# Create user assigned managed identity for AKS and role assignment
echo "Creating managed identity: ${AKS_ID_NAME}"
UMI=$(az identity create -g ${RESOURCE_GROUP} -n ${AKS_ID_NAME})

UMI_RESOURCE_ID=$(echo ${UMI} | jq -r '. | .id')
UMI_PRINCIPAL_ID=$(echo ${UMI} | jq -r '. | .principalId')

# Assign "Network Contributor" role to AKS identity
Role=$(az role assignment create --role "Network Contributor" --assignee-principal-type ServicePrincipal --assignee-object-id ${UMI_PRINCIPAL_ID})

# Create aks cluster
if [ "$NETWORK_PLUGIN" == "kubenet" ]; then
    NETWORK_PROFILE="--network-plugin kubenet --pod-cidr ${KUBENET_POD_CIDR}"
    NODEPOOL_PROFILE="--max-pods 250"
elif [ "$NETWORK_PLUGIN" == "azure" ]; then
    NETWORK_PROFILE="--network-plugin azure"
    NODEPOOL_PROFILE="--pod-subnet-id ${SUBNET_POD_ID}"
else
    echo "Network plugin ${NETWORK_PLUGIN} is not supported, should be kubenet or azure"
fi

echo "Creating AKS cluster: ${AKS_CLUSTER_NAME}"
AKS=$(az aks create -n ${AKS_CLUSTER_NAME} -g ${RESOURCE_GROUP} -l ${LOCATION} \
    --enable-managed-identity --assign-identity ${UMI_RESOURCE_ID} \
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


# Creating ILB
echo "Creating ILB: ${LB_NAME}"
LB=$(az network lb create -g ${RESOURCE_GROUP} -n ${LB_NAME} --sku Standard --frontend-ip-name ${GW_VMSS_ID} --subnet ${SUBNET_GATEWAY_ID} --backend-pool-name ${GW_VMSS_ID})

LB_BACKEND_ID=$(echo ${LB} | jq -r '. | .loadBalancer | .backendAddressPools[0] | .id')
echo $LB_BACKEND_ID

# Updating Gateway VMSS backend to ILB backend
echo "Updating Gateway VMSS to bind to LB backend"
VMSS_UPDATE=$(az vmss update -g ${AKS_NODE_RESOURCE_GROUP} -n ${GW_VMSS_NAME} --add virtualMachineProfile.networkProfile.networkInterfaceConfigurations[0].ipConfigurations[0].loadBalancerBackendAddressPools \
    "{\"id\": \"${LB_BACKEND_ID}\"}")
VMSS_INSTANCE_UPDATE=$(az vmss update-instances -g ${AKS_NODE_RESOURCE_GROUP} -n ${GW_VMSS_NAME} --instance-ids '*')

echo "Test environment setup completed."