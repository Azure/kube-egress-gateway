# Installation Guide

## Prerequisites
* [Azure CLI](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli)
* [Helm 3](https://helm.sh/docs/intro/install/)
* A Kubernetes cluster on Azure with a dedicated nodepool backed by Azure Virtual Machine Scale Set (VMSS) with at least one node (VMSS instance).
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
3. Assign "Network Contributor" and "Virtual Machine Contributor" roles to the identity. kube-egress-gateway components need these two roles to configure Load Balancer, Public IP Prefix, and VMSS resources.
    ```
    networkResourceGroup="<network resource group>"
    vmssResourceGroup="<vmss resource group>"
    networkRGID="/subscriptions/<your subscriptionID>/resourceGroups/$networkResourceGroup"
    vmssRGID="/subscriptions/<your subscriptionID>/resourceGroups/$vmssResourceGroup"
    vmssID="/subscriptions/<your subscriptionID>/resourceGroups/$vmssResourceGroup/providers/Microsoft.Compute/virtualMachineScaleSets/<your gateway vmss>"
    
    # assign Network Contributor role on scope networkResourceGroup and vmssResourceGroup to the identity
    az role assignment create --role "Network Contributor" --assignee $identityClientId --scope $networkRGID
    az role assignment create --role "Network Contributor" --assignee $identityClientId --scope $vmssRGID
    
    # assign Virtual Machine Contributor role on scope gateway vmss to the identity
    az role assignment create --role "Virtual Machine Contributor" --assignee $identityClientId --scope $vmssID
    ```
4. Fill the identity clientID in your Azure cloud config file. See [sample_cloud_config_msi.yaml](samples/sample_azure_config_msi.yaml) for example.
    ```
    useManagedIdentityExtension: true
    userAssignedIdentityID: "$identityClientId"
    ```

### Use Service Principal
1. Create a service principal and assign Contributor role on network resource group and vmss resource group scopes. And you get sp clientID and secret from the output.
    ```
    appName="<app name>"
    networkRGID="/subscriptions/<your subscriptionID>/resourceGroups/$networkResourceGroup"
    vmssRGID="/subscriptions/<your subscriptionID>/resourceGroups/$vmssResourceGroup"
    az ad sp create-for-rbac -n $appName --role Contributor --scopes $networkRGID $vmssRGID
    ```
2. Fill the sp clientID and secret in your Azure cloud config file. See [sample_cloud_config_sp.yaml](samples/sample_azure_config_sp.yaml) for example.
    ```
    useManagedIdentityExtension: false
    aadClientId: "<sp clientID>"
    aadClientSecret: "<sp secret>"
    ```

## Install kube-egress-gateway as Helm Chart
See details [here](../helm/kube-egress-gateway/README.md).