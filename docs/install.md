# Installation Guide

## Prerequisites

* [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli)
* [Helm 3](https://helm.sh/docs/intro/install/)
* A Kubernetes cluster on Azure with a dedicated nodepool for gateway nodes with at least one node.
  * **For public IP egress (default)**: Nodepool backed by Azure Virtual Machine Scale Set (VMSS)
  * **For private IP egress (preview, AKS 1.34+)**: Nodepool backed by standard Azure VMs (Availability Set mode) for stable private IP assignment
  * The nodes should be tainted with `kubeegressgateway.azure.com/mode=true:NoSchedule` so that no other workload can land on.
  * The nodes should be labeled with `kubeegressgateway.azure.com/mode: true` so that kube-egress-gateway DaemonSets NodeSelector can identify these.
  * The nodes should be linux only.
  * The nodes should be excluded from cluster autoscaler.

## Create Azure credentials
kube-egress-gateway components communicates with Azure Resource Manager (ARM) to manipulate Azure resources. It needs an identity to access the APIs. kube-egress-gateway supports both [Managed Identity](https://learn.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/overview) and [Service Principal](https://learn.microsoft.com/en-us/cli/azure/create-an-azure-service-principal-azure-cli).

### Use UserAssigned Managed Identity
1. Create a UserAssigned managed identity. This identity can be created in any resource group as long as permissions are set correctly.
    ```
    identityName="<identity name>"
    resourceGroup="<resource group>"
    az identity create -g $resourceGroup -n $identityName
    ```
2. Retrieve the `identityID` and `clientID` from the identity you just created.
    ```
    identityClientId=$(az identity show -g $resourceGroup -n $identityName -o tsv --query "clientId")
    identityId=$(az identity show -g $resourceGroup -n $identityName -o tsv --query "id")
    ```
3. Assign "Network Contributor" and "Virtual Machine Contributor" roles to the identity. kube-egress-gateway components need these two roles to configure Load Balancer, Public IP Prefix (if using public IPs), and VMSS/VM resources.
    ```
    networkResourceGroup="<network resource group>"
    gatewayResourceGroup="<gateway nodepool resource group>"
    networkRGID="/subscriptions/<your subscriptionID>/resourceGroups/$networkResourceGroup"
    gatewayRGID="/subscriptions/<your subscriptionID>/resourceGroups/$gatewayResourceGroup"
    gatewayID="/subscriptions/<your subscriptionID>/resourceGroups/$gatewayResourceGroup/providers/Microsoft.Compute/virtualMachineScaleSets/<your gateway vmss>"
    # OR for VM-based nodepools (private IP mode):
    # gatewayID="/subscriptions/<your subscriptionID>/resourceGroups/$gatewayResourceGroup/providers/Microsoft.Compute/availabilitySets/<your gateway availabilitySet>"

    # assign Network Contributor role on scope networkResourceGroup and gatewayResourceGroup to the identity
    az role assignment create --role "Network Contributor" --assignee $identityClientId --scope $networkRGID
    az role assignment create --role "Network Contributor" --assignee $identityClientId --scope $gatewayRGID

    # assign Virtual Machine Contributor role on scope gateway nodepool to the identity
    az role assignment create --role "Virtual Machine Contributor" --assignee $identityClientId --scope $gatewayID
    ```
4. Fill the identity clientID in your Azure cloud config file. See [sample_cloud_config_msi.yaml](samples/sample_azure_config_msi.yaml) for example.
    ```
    useManagedIdentityExtension: true
    userAssignedIdentityID: "$identityClientId"
    ```

### Use Service Principal

1. Create a service principal and assign Contributor role on network resource group and gateway nodepool resource group scopes. And you get sp clientID and secret from the output.
    ```
    appName="<app name>"
    networkRGID="/subscriptions/<your subscriptionID>/resourceGroups/$networkResourceGroup"
    gatewayRGID="/subscriptions/<your subscriptionID>/resourceGroups/$gatewayResourceGroup"
    az ad sp create-for-rbac -n $appName --role Contributor --scopes $networkRGID $gatewayRGID
    ```
2. Fill the sp clientID and secret in your Azure cloud config file. See [sample_cloud_config_sp.yaml](samples/sample_azure_config_sp.yaml) for example.
    ```
    useManagedIdentityExtension: false
    aadClientId: "<sp clientID>"
    aadClientSecret: "<sp secret>"
    ```

## Install kube-egress-gateway as Helm Chart
See details [here](../helm/kube-egress-gateway/README.md). 