# kube-egress-gateway Helm Chart

This Helm chart enables installation and maintenance of Azure kube-egress-gateway project. The provided components and compatible with Kubernetes 1.16 and higher.

# Defaults

## Installation

A Helm repo is maintained at the following URI:

- https://raw.githubusercontent.com/Azure/kube-egress-gateway/main/helm/repo

The `kube-egress-gateway` project relies on [cert-manager](https://cert-manager.io/) to provide webhook certificate issuance and rotation. You can install it separately (recommended) or install it as a subchart included in this Helm chart.

### Option 1 (RECOMMENDED)

We recommend users to install cert-manager before installing this chart. 

To manually install cert-manager, you can follow [cert-manager official doc](https://cert-manager.io/docs/installation/helm/#option-1-installing-crds-with-kubectl).

You can install CRD resources with `kubectl` first:

```bash
$ kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.crds.yaml
```

And then install cert-manager Helm chart:
```bash
$ helm install \
  cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --version v1.11.0 \
  # --set installCRDs=true
```

Then to install `kube-egress-gateway`, you may run below `helm` command:

```bash
$ helm install --repo https://raw.githubusercontent.com/Azure/kube-egress-gateway/main/helm/repo kube-egress-gateway --set common.imageRepository=<TBD> --set common.imageTag=<TBD> -f azure_config.yaml
```

See more details about azure_config.yaml in [azure cloud configurations](#azure-cloud-configurations)

### Option 2

You may also install cert-manager as a subchart with `enabled` set to `true`:

```bash
$ helm install \
  --repo https://raw.githubusercontent.com/Azure/kube-egress-gateway/main/helm/repo kube-egress-gateway \
  --set common.imageRepository=<TBD> \
  --set common.imageTag=<TBD> \
  --set cert-manager.enabled=true \ # to enable cert-manager installation
  --set cert-manager.namespace=<your desired namespace> \ # to install cert-manager components in specified namespace, default to cert-manager
  -f azure_config.yaml
```
## Uninstallation

Use the following commans to find the `kube-egress-gateway` Helm chart release name and uninstall it.

```bash
$ helm list
$ helm delete <kube-egress-gateway-char-release-name>
```

# Configurable values

Below we provide a list of configurations you can set when invoking `helm install` against your Kubernetes cluster.

## azure cloud configurations

Azure cloud configuration provides information including resource metadata and credentials for gatewayControllerManager and gatewayDaemonManager to manipulate Azure resources. It's embedded into a Kubernetes secret and mounted to the pods. The complete configuration is **required** for the components to run. 

| configuration value | description | Remark |
| --- | --- | --- |
| `config.azureCloudConfig.cloud` | The cloud where Azure resources belong. Choose from `AzurePublicCloud`, `AzureChinaCloud`, and `AzureGovernmentCloud`. | Required, helm chart defaults to `AzurePublicCloud` |
| `config.azureCloudConfig.tenantId` | The AAD Tenant ID for the subscription where the Azure resources are deployed. | |
| `config.azureCloudConfig.subscriptionId` | The ID of the subscription where Azure resources are deployed. | |
| `config.azureCloudConfig.useUserAssignedIdentity` | Boolean indicating whether or not to use a managed identity. | `true` or `false` |
| `config.azureCloudConfig.userAssignedIdentityID` | ClientID of the user-assigned managed identity with RBAC access to Azure resources. | Required to use user assigned managed identity. |
| `config.azureCloudConfig.aadClientId` | The ClientID for an AAD application with RBAC access to Azure resources. | Required if `useUserAssignedIdentity` is set to `false`. |
| `config.azureCloudConfig.aadClientSecret` | The ClientSecret for an AAD application with RBAC access to Azure resources. | Required if `useUserAssignedIdentity` is set to `false`. |
| `config.azureCloudConfig.resourceGroup` | The name of the resource group where cluster resources are deployed. | |
| `config.azureCloudConfig.userAgent` | The userAgent provided to Azure when accessing Azure resources. | |
| `config.azureCloudConfig.location` | The azure region where resource group and its resources is deployed. | |
| `config.azureCloudConfig.loadBalancerName` | The name of the load balancer in front of gateway VMSS for high availability. | Required, helm chart defaults to `gateway-ilb`. |
| `config.azureCloudConfig.loadBalancerResourceGroup` | The resouce group where the load balancer to be deployed. | Optional. If not provided, it's the same as `config.azureCloudConfig.resourceGroup`. |
| `config.azureCloudConfig.vnetName` | The name of the virtual network where load balancer frontend ip comes from. | |
| `config.azureCloudConfig.vnetResourceGroup` | The resource group where the virtual network is deployed. | Optional. If not set, it's the same as `config.azureCloudConfig.resourceGroup`. |
| `config.azureCloudConfig.subnetName` | The name of the subnet inside the virtual network where the load balancer frontend ip comes from. | |

You can create a file `azure.yaml` with the following content, and pass it to `helm install` command: `helm install <release-name> <chart-name> -f azure.yaml`

```yaml
config:
  azureCloudConfig:
    cloud: "AzurePublicCloud"
    tenantId: "00000000-0000-0000-0000-000000000000"
    subscriptionId: "00000000-0000-0000-0000-000000000000"
    useUserAssignedIdentity: false
    userAssignedIdentityID: "00000000-0000-0000-0000-000000000000"
    aadClientId: "00000000-0000-0000-0000-000000000000"
    aadClientSecret: "<your secret>"
    userAgent: "kube-egress-gateway-controller"
    resourceGroup: "<resource group name>"
    location: "<resource group location>"
    loadBalancerName: "gateway-ilb"
    loadBalancerResourceGroup: ""
    vnetName: "<virtual network name>"
    vnetResourceGroup: ""
    subnetName: "<subnet name>"
```

## common configurations

The Helm chart installs 5 components with different images: gateway-controller-manager, gateway-daemon-manager, gateway-CNI-manager, gateway-CNI, and gateway-CNI-Ipam. For easy deployment, we provide `common.imageRepository` and `common.imageTag`. If all images can be downloaded from the same reposity (use `common.imageRepository`) and come from the same build/release (use `common.imageTag`), you can set just these two values instead of each individual value as shown in the following sections. If for any particular components, you want to apply a different repository/tag, you can specify in their own configurations, overriding these two values.

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
| `gatewayCNIManager.exceptionCidrs` | `[""]` | A list of cidrs that should be exempted from all egress gateways, e.g. intra-cluster traffic. |
| `gatewayCNIManager.cniConfigFileName` | `01-egressgateway.conflist` | Name of the newly generated cni configuration list file. |

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

## cert-manager configurations

`kube-egress-gateway` relies on `cert-manager` to provide webhook certificate issuance and renewal. For configurations to cert-manager itself, please refer to [cert-manager official website](https://cert-manager.io/docs/installation/helm/). By default cert-manager subchart is disabled. To enable, add `--set cert-manager.enabled=true` in your `helm install` command. To deploy cert-manager components in a specific namespace, add `--set cert-manager.namespace=<your desired namespace>` in your `helm install` command. The default namespace is `cert-manager`.
