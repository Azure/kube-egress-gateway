# bicep templates to deploy a test environment

This folder contains a simple bicep template to deploy test environment. To deploy via Azure CLI, run below sample command:
```
az deployment group create \
   -n <DeploymentName> \
   -g <ResourceGroupName> \
   -f gateway.bicep \
   --param adminSSHKey="$(cat ~/.ssh/id_rsa.pub)"
```

The script would create one AKS cluster with two nodepools, one as normal Kubernetes workload nodepool and one as gateway nodepool, and one internal Load Balancer named `<gatewayNodepoolName-ilb>`. The LB does not have any sub-resources. Both gateway nodepool and LB are to be configured by controllers. 