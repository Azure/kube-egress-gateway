# Static Egress Gateway

[![Build Status](https://msazure.visualstudio.com/CloudNativeCompute/_apis/build/status%2FAKS%2Fkube-egress-gateway%2FAzure.kube-egress-gateway-e2e?branchName=main)](https://msazure.visualstudio.com/CloudNativeCompute/_build/latest?definitionId=319204&branchName=main)
[![Coverage Status](https://coveralls.io/repos/github/Azure/kube-egress-gateway/badge.svg)](https://coveralls.io/github/Azure/kube-egress-gateway)

Static Egress Gateway is a scalable and cost-efficient solution to configure fixed source IP addresses for outbound traffic from your Azure Kubernetes Service (AKS) workloads. 

![Kube Egress Gateway](docs/images/kube_egress_gateway.png)

## Documentation

See the official [Static Egress Gateway documentation](https://learn.microsoft.com/en-us/azure/aks/configure-static-egress-gateway) for details on:
- How to enable the feature in your AKS cluster.
- How to configure egress traffic by creating the necessary custom resources and pod annotations.
- The feature's limitations.

While the documentation above describes the recommended installation procedure, if you would like to instead deploy the components yourself using Helm, follow this [installation guide](docs/install.md).

## Design

* The [design doc](docs/design.md) offers details on Static Egress Gateway's internals.

## Troubleshooting

Refer to [troubleshooting guide and known issues](docs/troubleshooting.md).

## Contributing

This project welcomes contributions and suggestions.  Most contributions require you to agree to a
Contributor License Agreement (CLA) declaring that you have the right to, and actually do, grant us
the rights to use your contribution. For details, visit <https://cla.opensource.microsoft.com>.

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