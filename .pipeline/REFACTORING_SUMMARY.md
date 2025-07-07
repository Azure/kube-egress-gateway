# Refactoring Summary: Azure Test Environment Deployment with Bicep

## Overview
Successfully refactored the ### Technical Notes

### AKS Cluster and Node Pool Deployment
- Both system and gateway node pools are deployed together with the cluster in a single atomic operation
- This improves deployment reliability and reduces deployment time
- Gateway node pool includes proper labels and taints for workload isolation

### SSH Key Handlingre test environment deployment script to use Bicep templates instead of imperative Azure CLI commands. This provides Infrastructure as Code (IaC) benefits including better maintainability, reusability, and declarative resource management.

## Changes Made

### 1. Created Bicep Templates
- **Main template** (`main.bicep`): Orchestrates all resources
- **VNet module** (`modules/vnet.bicep`): Virtual network and subnets
- **Identities module** (`modules/identities.bicep`): Managed identities
- **AKS module** (`modules/aks.bicep`): AKS cluster with system and gateway node pools deployed together
- **Role assignments modules**: RBAC configurations

### 2. Updated Deployment Script
- Replaced individual `az` commands with single `az deployment group create`
- Added parameter file generation using `envsubst`
- Maintained all original functionality including post-deployment configurations
- Improved error handling and output parsing

### 3. Added Supporting Files
- **Parameters template** (`parameters.json`): Environment variable substitution
- **Validation script** (`validate-bicep.sh`): Template validation and what-if preview
- **Cleanup script** (`cleanup-testenv.sh`): Resource group cleanup
- **Documentation** (`README.md`): Comprehensive usage guide

## File Structure
```
.pipeline/
├── bicep/
│   ├── main.bicep              # Main orchestration template
│   ├── parameters.json         # Parameter template
│   ├── README.md              # Documentation
│   └── modules/
│       ├── vnet.bicep         # Virtual network
│       ├── identities.bicep   # Managed identities
│       ├── aks.bicep          # AKS cluster
│       ├── roleAssignments.bicep      # All role assignments
│       └── roleAssignment.bicep       # Reusable role assignment module
└── scripts/
    ├── deploy-testenv.sh      # Updated deployment script
    ├── cleanup-testenv.sh     # Cleanup script
    └── validate-bicep.sh      # Validation script
```

## Benefits Achieved

### Infrastructure as Code
- **Declarative**: Resources defined in desired state
- **Version controlled**: Templates can be versioned and tracked
- **Reusable**: Parameterized templates for different environments
- **Consistent**: Same infrastructure deployed every time

### Improved Maintainability
- **Modular**: Separated concerns into focused modules
- **Readable**: Clear resource relationships and dependencies
- **Validated**: Built-in syntax and resource validation
- **Error reporting**: Better error messages and debugging

### Operational Benefits
- **Idempotent**: Safe to run multiple times
- **Dependencies**: Automatic resource dependency management
- **Rollback**: Azure Resource Manager handles failed deployments
- **What-if**: Preview changes before deployment

## Preserved Functionality
All original script functionality is maintained:
- Environment variable configuration
- Random region selection
- Resource group creation
- Network plugin support (kubenet, azure, overlay, cilium, calico)
- Gateway node pool with proper labels and taints
- RBAC role assignments
- VMSS tagging
- Azure configuration file generation
- Kubeconfig retrieval

## Usage

### Standard Deployment
```bash
# Set required environment variables
export AZURE_SUBSCRIPTION_ID="your-subscription-id"
export AZURE_TENANT_ID="your-tenant-id"

# Optional: customize deployment
export RESOURCE_GROUP="my-test-rg"
export LOCATION="eastus2"
export NETWORK_PLUGIN="overlay"

# Deploy
cd .pipeline/scripts
./deploy-testenv.sh
```

### Validation and Preview
```bash
# Validate templates
./validate-bicep.sh

# Preview changes (if resource group exists)
./validate-bicep.sh  # Follow prompts for what-if
```

### Cleanup
```bash
./cleanup-testenv.sh
```

## Technical Notes

### SSH Key Handling
- Uses generated SSH key in the template for simplicity
- Could be enhanced to accept custom SSH keys as parameter
- AKS cluster uses `--generate-ssh-keys` equivalent functionality

### Role Assignments
- Consolidated all role assignments into a single `roleAssignments.bicep` file using a reusable module pattern
- Created generic `roleAssignment.bicep` module for cross-scope deployments
- Main resource group roles assigned via Bicep
- Node resource group roles use same reusable module with different scope
- VM Contributor role for VMSS assigned post-deployment (due to dynamic VMSS name)

### Network Plugin Support
- Template dynamically configures network profile based on plugin choice
- Supports all original plugins: kubenet, azure, azure-podsubnet, overlay, cilium, calico
- Maintains original maxPods and subnet configurations

### Dependencies
- Bicep automatically manages resource dependencies
- Explicit dependency on AKS completion for role assignments
- Post-deployment script handles VMSS-specific configurations

## Validation Status
✅ All Bicep templates validated successfully
✅ All original functionality preserved
✅ Scripts tested and executable
✅ Documentation complete
