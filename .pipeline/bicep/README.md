# Bicep-based Test Environment Deployment

This directory contains Bicep templates and scripts for deploying the kube-egress-gateway test environment on Azure.

## Structure

```
bicep/
├── main.bicep                  # Main template that orchestrates all resources
├── parameters.json             # Parameter template (uses environment variables)
└── modules/
    ├── vnet.bicep             # Virtual network and subnets
    ├── identities.bicep       # Managed identities
    ├── aks.bicep              # AKS cluster and node pools
    ├── roleAssignments.bicep  # All role assignments (main and node RG)
    └── roleAssignment.bicep   # Reusable role assignment module
```

## Prerequisites

- Azure CLI installed and authenticated
- Bicep CLI installed (or use Azure CLI with Bicep extension)
- `jq` and `envsubst` utilities
- Required environment variables:
  - `AZURE_SUBSCRIPTION_ID`
  - `AZURE_TENANT_ID`

## Environment Variables

The following environment variables can be set to customize the deployment:

| Variable | Default | Description |
|----------|---------|-------------|
| `RESOURCE_GROUP` | `kube-egress-gw-rg` | Resource group name |
| `VNET_NAME` | `gateway-vnet` | Virtual network name |
| `AKS_ID_NAME` | `gateway-aks-id` | AKS control plane identity name |
| `AKS_KUBELET_ID_NAME` | `gateway-aks-kubelet-id` | AKS kubelet identity name |
| `AKS_CLUSTER_NAME` | `aks` | AKS cluster name |
| `NETWORK_PLUGIN` | `overlay` | Network plugin (kubenet/azure/azure-podsubnet/overlay/cilium/calico) |
| `DNS_SERVER_IP` | `10.245.0.10` | DNS server IP for AKS |
| `POD_CIDR` | `10.244.0.0/16` | Pod CIDR |
| `SERVICE_CIDR` | `10.245.0.0/16` | Service CIDR |
| `LB_NAME` | `kubeegressgateway-ilb` | Load balancer name |
| `LOCATION` | Random from (`eastus2`, `northeurope`, `westus2`) | Azure region |
| `KUBECONFIG_FILE` | - | Optional: kubeconfig file path |

## Usage

Run the deployment script:

```bash
cd .pipeline/scripts
./deploy-testenv.sh
```

## What Gets Deployed

1. **Resource Group** - Created if it doesn't exist
2. **Virtual Network** - With three subnets:
   - `gateway` (10.243.0.0/23) - For gateway nodes
   - `aks` (10.243.4.0/22) - For AKS system nodes
   - `pod` (10.243.8.0/22) - For pod subnet networking (when using azure-podsubnet)
3. **Managed Identities** - Control plane and kubelet identities
4. **AKS Cluster** - With system node pool and gateway node pool (deployed together)
5. **Role Assignments** - Network and VM contributor roles
6. **Configuration Files** - `azure.json` for controllers

## Network Plugin Support

The template supports multiple AKS network plugins:

- **kubenet**: Basic kubenet networking with pod CIDR
- **azure**: Azure CNI with default settings
- **azure-podsubnet**: Azure CNI with dedicated pod subnet
- **overlay**: Azure CNI overlay mode
- **cilium**: Azure CNI overlay with Cilium dataplane
- **calico**: Azure CNI with Calico network policies

## Post-Deployment

After deployment, the script:
1. Tags the gateway VMSS with `aks-managed-gatewayIPPrefixSize=31`
2. Assigns VM Contributor role to the kubelet identity for the gateway VMSS
3. Generates `azure.json` configuration file
4. Optionally retrieves kubeconfig

## Benefits of Bicep Approach

- **Declarative**: Infrastructure as Code with better maintainability
- **Idempotent**: Safe to run multiple times
- **Modular**: Reusable components
- **Validation**: Built-in syntax and resource validation
- **Dependencies**: Automatic dependency management
- **Outputs**: Structured output handling
- **Error Handling**: Better error reporting than imperative scripts

## Troubleshooting

If deployment fails:

1. Check that all required environment variables are set
2. Verify Azure CLI authentication: `az account show`
3. Ensure subscription has sufficient quotas for Standard_DS2_v2 VMs
4. Check deployment logs in Azure portal under Resource Group > Deployments
5. Use `az deployment group create --debug` for verbose output
