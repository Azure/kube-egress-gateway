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

## Configuration Options

### Public IP vs Private IP Egress

kube-egress-gateway always provisions private IPs on virtual machines gateway nodes as secondary IP configurations. The `provisionPublicIps` setting controls whether *additional* public IP prefix resources are created and associated with these private IPs. This gives you flexibility in how pods egress:

#### Public IP Mode (Default)

In this mode (`provisionPublicIps: true`), public IP prefix resources are created and associated with the gateway nodes, enabling pods to access the Internet with fixed public egress IPs.

**Use cases:**
* Pods need to access external Internet services
* External systems require allowlisting specific public IP addresses
* Outbound Internet connectivity with a fixed, predictable source IP is required

**Configuration:**
```yaml
spec:
  gatewayVmssProfile:
    vmssResourceGroup: myResourceGroup
    vmssName: myGatewayVMSS
    publicIpPrefixSize: 31
  provisionPublicIps: true  # Default value
  publicIpPrefixId: /subscriptions/.../publicIPPrefixes/myPIPPrefix  # Optional BYO
```

#### Private IP Only Mode

In this mode (`provisionPublicIps: false`), no public IP prefix resources are created. Pods egress using only the private IPs that are allocated on the gateway nodes.

**Requirements:**
* **VM-based gateway nodepool** (`gatewayNodepoolName`) is **required** - This mode is NOT supported with VMSS-based gateways (`gatewayVmssProfile`)
* **Kubernetes version ≥ 1.34** is required for VM gateway pool support
* Properly sized gateway subnet (see subnet requirements below)

> **Why VM gateway pools for private IP mode?**
>
> Private static IP egress requires deterministic IP assignment that remains stable across node scale operations and upgrades. VM-based gateway pools achieve this through:
> * **Pre-created NICs**: NICs are created upfront (up to the pool's max size), allowing the controller to write secondary `ipConfigurations[]` that pin private IPs to specific nodes
> * **IP stability**: Secondary IPs stick with each node across restarts and are preserved through rolling operations, avoiding source IP drift
> * **Predictable state**: This design enables validation that secondary IPs remain unchanged after upgrades, critical for allowlisting scenarios
>
> In contrast, VMSS-based pools were designed for dynamic ipConfigs tied to public IP prefixes and don't provide the same IP stability guarantees.


**Configuration:**
```yaml
spec:
  # VM-based gateway nodepool required for private IP only mode
  gatewayNodepoolName: myGatewayNodepool
  
  # Set to false to skip public IP provisioning
  provisionPublicIps: false
```

**Provisioning process:**

When you create or update a StaticGatewayConfiguration with `provisionPublicIps: false`, the controller:

1. Checks/creates the internal load balancer (ILB) and backend pool
2. Ensures each selected gateway node has the secondary private IP in its NIC `ipConfigurations[]`
3. Updates the StaticGatewayConfiguration status with assigned private IP addresses

**Verifying IP assignment:**

After deployment, verify the assigned private IPs in the StaticGatewayConfiguration status:

```bash
kubectl get staticgatewayconfigurations -n <namespace> <name> -o jsonpath='{.status.egressIpPrefix}'
```

The `egressIpPrefix` field will show a comma-separated list of private IPs (e.g., `10.0.1.8,10.0.1.9`). These IPs remain stable across node operations.

**Important considerations:**
* When using `gatewayNodepoolName` (VM-based gateway nodepools), you have the flexibility to set `provisionPublicIps` to false
* Ensure your network configuration (ExpressRoute, VPN, Private Link, VNet peering) is properly set up to route traffic to the intended private destinations
* No public IP prefix resources will be created
* The `publicIpPrefixId` field cannot be used when `provisionPublicIps` is false 