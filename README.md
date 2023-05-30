# Project kube-egress-gateway

## Table of Contents

<!-- toc -->
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
- [Design](#design)
  - [Components](#components)
  - [Test Plan](#test-plan)
    - [Needed Tests](#needed-tests)
- [Current Progress](#current-progress)
- [Contributing](#contributing)
- [Trademarks](#trademarks)
<!-- /toc -->

## Summary

kube-egress-gateway provides the ability to configure fixed source IP for Kubernetes pod egress traffic on Azure.

## Motivation

Currently, the only way to provide fixed egress IP for pods is to deploy a nodepool with dedicated NAT gateway/ILPIP and schedule pods only to this nodepool, which may not be cost-efficient if there may just be a few pods and different pods require different egress IPs.

Here we propose another method where a dedicated nodepool is used as gateway. Pods with static egress requirement are still scheduled on regular cluster nodes but get their traffic routed to the gateway nodepool while other pods remain unaffected.

### Goals

* Provide a scalable way for users to have fixed egress IPs on certain workloads.

## Design

This approach utilizes a dedicated AKS nodepool or Azure virtual machine scale set (VMSS) behind an Azure internal load balancer (ILB) as gateway. Please refer to `tests/deploy` to create a sample environment where an AKS cluster along with a gateway nodepool and ILB are created.

When the gateway nodepool is created, there are no gateway IPConfigurations. A user should create a namespaced `StaticGatewayConfiguration` CRD object to create an egress gateway configuration, specifying the target gateway nodepool, and optional BYO public IP prefix. Users can then annotate the pod (`kubernetes.azure.com/static-gateway-configuration: <gateway config name, e.g. aks-static-gw-001>`) to claim a static gateway configuration as egress.

### Components
* **Gateway manager**: Contains three controllers, one monitors `StaticGatewayConfiguration` CR and generates corresponding `GatewayLBConfiguration` and `GatewayVMConfiguration` CRs. Two other controllers monitor these two CRs and configure Azure LB and Azure VMSS respectively.
* **Gateway daemonSet on gateway nodes**: Monitors `StaticGatewayConfiguration` CR and `PodWireguardEndpoint` CR, configures the network namespaces, interfaces, and routes on gateway nodes.
* **Chained CNI and pod egress controller daemonSet on regular cluster nodes**: Setups wireguard interfaces and routes in pods' network namespace.

### Test Plan

#### Needed Tests

- Unit tests
- E2E tests to allow `StaticEgressGateway` CRUD and pod annotation, and make sure static egress gateway works.

## Current Progress
- [] StaticGatewayConfiguration operator - Wantong
- [] GatewayWireguardEndpoint CRD, PodWireguardEndpoint CRD, and Gateway nodepool daemonSet 
- [] Chained CNI and non-gateway nodepool daemonSet
- [] E2E tests

<!-- ### Graduation Criteria
## Proposed roadmap
## Implementation History -->

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit https://cla.opensource.microsoft.com.

When you submit a pull request, a CLA bot will automatically determine whether you need to provide
a CLA and decorate the PR appropriately (e.g., status check, comment). Simply follow the instructions
provided by the bot. You will only need to do this once across all repos using our CLA.

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/).
For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or
contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.

## Trademarks

This project may contain trademarks or logos for projects, products, or services. Authorized use of Microsoft 
trademarks or logos is subject to and must follow 
[Microsoft's Trademark & Brand Guidelines](https://www.microsoft.com/en-us/legal/intellectualproperty/trademarks/usage/general).
Use of Microsoft trademarks or logos in modified versions of this project must not cause confusion or imply Microsoft sponsorship.
Any use of third-party trademarks or logos are subject to those third-party's policies.

test