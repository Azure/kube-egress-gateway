# kube-egress-gateway Helm Chart

This Helm chart enables installation and maintenance of Azure kube-egress-gateway project. The provided components and compatible with Kubernetes 1.16 and higher.

# Defaults

## Installation

Clone this repository, kube-egress-gateway chart is maintained in `helm/kube-egress-gateway` directory:
```
git clone https://github.com/Azure/kube-egress-gateway.git
```

To install `kube-egress-gateway`, you may run below `helm` command:

```bash
$ helm install \
  kube-egress-gateway ./helm/kube-egress-gateway \
  --namespace kube-egress-gateway-system \
  --create-namespace \
  --set common.imageRepository=mcr.microsoft.com/aks \
  --set common.imageTag=v0.0.9 \ 
  -f azure_config.yaml
```

See more details about azure_config.yaml in [azure cloud configurations](#azure-cloud-configurations)

# Configurable values

Below we provide a list of configurations you can set when invoking `helm install` against your Kubernetes cluster. You can also check [values.yaml](./values.yaml).

## azure cloud configurations

Azure cloud configuration provides information including resource metadata and credentials for gatewayControllerManager and gatewayDaemonManager to manipulate Azure resources. It's embedded into a Kubernetes secret and mounted to the pods. The complete configuration is **required** for the components to run. 

| configuration value                                   | description | Remark                                                                               |
|-------------------------------------------------------| --- |--------------------------------------------------------------------------------------|
| `config.azureCloudConfig.cloud`                       | The cloud where Azure resources belong. Choose from `AzurePublicCloud`, `AzureChinaCloud`, and `AzureGovernmentCloud`. | Required, helm chart defaults to `AzurePublicCloud`                                  |
| `config.azureCloudConfig.tenantId`                    | The AAD Tenant ID for the subscription where the Azure resources are deployed. |                                                                                      |
| `config.azureCloudConfig.subscriptionId`              | The ID of the subscription where Azure resources are deployed. |                                                                                      |
| `config.azureCloudConfig.useManagedIdentityExtension` | Boolean indicating whether or not to use a managed identity. | `true` or `false`                                                                    |
| `config.azureCloudConfig.userAssignedIdentityID`      | ClientID of the user-assigned managed identity with RBAC access to Azure resources. | Required to use managed identity.                                                    |
| `config.azureCloudConfig.aadClientId`                 | The ClientID for an AAD application with RBAC access to Azure resources. | Required if `useManagedIdentityExtension` is set to `false`.                         |
| `config.azureCloudConfig.aadClientSecret`             | The ClientSecret for an AAD application with RBAC access to Azure resources. | Required if `useManagedIdentityExtension` is set to `false`.                         |
| `config.azureCloudConfig.resourceGroup`               | The name of the resource group where cluster resources are deployed. |                                                                                      |
| `config.azureCloudConfig.userAgent`                   | The userAgent provided to Azure when accessing Azure resources. |                                                                                      |
| `config.azureCloudConfig.location`                    | The azure region where resource group and its resources is deployed. |                                                                                      |
| `config.azureCloudConfig.gatewayLoadBalancerName`     | The name of the load balancer in front of gateway VMSS for high availability. | Required, helm chart defaults to `kubeegressgateway-ilb`.                            |
| `config.azureCloudConfig.loadBalancerResourceGroup`   | The resouce group where the load balancer to be deployed. | Optional. If not provided, it's the same as `config.azureCloudConfig.resourceGroup`. |
| `config.azureCloudConfig.vnetName`                    | The name of the virtual network where load balancer frontend ip comes from. |                                                                                      |
| `config.azureCloudConfig.vnetResourceGroup`           | The resource group where the virtual network is deployed. | Optional. If not set, it's the same as `config.azureCloudConfig.resourceGroup`.      |
| `config.azureCloudConfig.subnetName`                  | The name of the subnet inside the virtual network where the load balancer frontend ip comes from. |                                                                                      |

You can create a file `azure.yaml` with the following content, and pass it to `helm install` command: `helm install <release-name> <chart-name> -f azure.yaml`

```yaml
config:
  azureCloudConfig:
    cloud: "AzurePublicCloud"
    tenantId: "00000000-0000-0000-0000-000000000000"
    subscriptionId: "00000000-0000-0000-0000-000000000000"
    useManagedIdentityExtension: false
    userAssignedIdentityID: "00000000-0000-0000-0000-000000000000"
    aadClientId: "00000000-0000-0000-0000-000000000000"
    aadClientSecret: "<your secret>"
    userAgent: "kube-egress-gateway-controller"
    resourceGroup: "<resource group name>"
    location: "<resource group location>"
    gatewayLoadBalancerName: "kubeegressgateway-ilb"
    loadBalancerResourceGroup: ""
    vnetName: "<virtual network name>"
    vnetResourceGroup: ""
    subnetName: "<subnet name>"
```

## common configurations

The Helm chart installs 5 components with different images: gateway-controller-manager, gateway-daemon-manager, gateway-CNI-manager, gateway-CNI, and gateway-CNI-Ipam. For easy deployment, we provide `common.imageRepository` and `common.imageTag`. If all images can be downloaded from the same reposity (use `common.imageRepository`) and come from the same build/release (use `common.imageTag`), you can set just these two values instead of each individual value as shown in the following sections. If for any particular components, you want to apply a different repository/tag, you can specify in their own configurations, overriding these two values.

Additionally, `common.gatewayLbProbePort` defines the gateway LoadBalancer probe port which is consumed by both gateway-controller-manager (LB probe creator) and gateway-daemon-manager (probe server). The default value is `8082`.

## gateway-controller-manager configurations

| configuration value | default value | description |
| --- | --- | --- |
| `gatewayControllerManager.enabled` | `true` | Enable or disable the gatewayControllerManager deployment. |
| `gatewayControllerManager.imageRepository` | | Container image repository containing gatewayControllerManager image. |
| `gatewayControllerManager.imageName` | `kube-egress-gateway-cni` | Name of gatewayControllerManager image. |
| `gatewayControllerManager.imageTag` | | Tag of gatewayControllerManager image. |
| `gatewayControllerManager.imagePullPolicy` | `IfNotPresent` | Image pull policy for gatewayControllerManager's image. |
| `gatewayControllerManager.replicas` | `1` | Number of replicas for gatewayControllerManager deployment. By default, just 1 runs in the cluster. To improve availability, choose number greater than 1. |
| `gatewayControllerManager.leaderElect` | `true` | If multiple relicas are enabled for gatewayControllerManager, enable or disable leader Election among the relicas. Default to `true`. |
| `gatewayControllerManager.metricsBindPort` | `8080` | Port that gatewayControllerManager listens on for `/metrics` requests. |
| `gatewayControllerManager.healthProbeBindPort` | `8081` | Port that gatewayControllerManager listens on for health probe requests. |

## gateway-daemon-manager configurations

| configuration value | default value | description |
| --- | --- | --- |
| `gatewayDaemonManager.enabled` | `true` | Enable or disable the gatewayDaemonManager daemonSet on gateway nodes. |
| `gatewayDaemonManager.imageRepository` | | Container image repository containing gatewayDaemonManager image. |
| `gatewayDaemonManager.imageName` | `kube-egress-gateway-cni` | Name of gatewayDaemonManager image. |
| `gatewayDaemonManager.imageTag` | | Tag of gatewayDaemonManager image. |
| `gatewayDaemonManager.imagePullPolicy` | `IfNotPresent` | Image pull policy for gatewayDaemonManager's image. |
| `gatewayDaemonManager.healthProbeBindPort` | `8081` | Port that gatewayDaemonManager listens on for health probe requests. Note: gatewayDaemonManager sets `hostNetwork` to true so it occupies gateway nodes' port directly. |

## gateway-CNI-manager configurations

| configuration value | default value | description |
| --- | --- | --- |
| `gatewayCNIManager.enabled` | `true` | Enable or disable the gatewayCNIManager daemonSet (together with gatewayCNI and gatewayCNI-Ipam) on regular work nodes. |
| `gatewayCNIManager.imageRepository` | | Container image repository containing gatewayCNIManager image. |
| `gatewayCNIManager.imageName` | `kube-egress-gateway-cni` | Name of gatewayCNIManager image. |
| `gatewayCNIManager.imageTag` | | Tag of gatewayCNIManager image. |
| `gatewayCNIManager.imagePullPolicy` | `IfNotPresent` | Image pull policy for gatewayCNIManager's image. |
| `gatewayCNIManager.grpcServerPort` | `50051` | Port which cniManager grpc server listens on. Also used for cniManager pod liveness and readiness probes. |
| `gatewayCNIManager.exceptionCidrs` | `[""]` | A list of cidrs that should be exempted from all egress gateways, e.g. intra-cluster traffic. |
| `gatewayCNIManager.cniConfigFileName` | `01-egressgateway.conflist` | Name of the newly generated cni configuration list file. |
| `gatewayCNIManager.cniUninstallConfigMapName` | `cni-uninstall` | Name of the configMap indicating whether cni plugin needs to be uninstalled upon gatewayCNIManager pod shutdown. |
| `gatewayCNIManager.cniUninstall` | `false` | Boolean indicating whether to uninstall kube-egress-gateway CNI plugin upon gatewayCNIManager pod shutdown. |

## gateway-CNI and gateway-CNI-Ipam configurations

| configuration value | default value | description |
| --- | --- | --- |
| `gatewayCNI.imageRepository` | | Container image repository containing gatewayCNI image. |
| `gatewayCNI.imageName` | `kube-egress-gateway-cni` | Name of gatewayCNI image. |
| `gatewayCNI.imageTag` | | Tag of gatewayCNI image. |
| `gatewayCNI.imagePullPolicy` | `IfNotPresent` | Image pull policy for gatewayCNI's image. |
| `gatewayCNIIpam.imageRepository` | | Container image repository containing gatewayCNI-Ipam image. |
| `gatewayCNIIpam.imageName` | `kube-egress-gateway-cni-ipam` | Name of gatewayCNI-Ipam image. |
| `gatewayCNIIpam.imageTag` | | Tag of gatewayCNI-Ipam image. |
| `gatewayCNIIpam.imagePullPolicy` | `IfNotPresent` | Image pull policy for gatewayCNI-Ipam's image. |
