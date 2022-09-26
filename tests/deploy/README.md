# bicep templates to deploy a test environment

This folder contains a simple bicep template to deploy test environment. To deploy via Azure CLI, run below sample command:
```
RESOURCE_GROUP=<resource group name> ./deploy-testenv.sh
```

The script would create one AKS cluster with two nodepools, one as normal Kubernetes workload nodepool and one as gateway nodepool, and one internal Load Balancer named `<gatewayNodepoolName-ilb>`.
The LB is preconfigured with one frontend ipConfiguration and one backend pool, filled with VMs from the gateway nodepool.