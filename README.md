# kube-egress-gateway

kube-egress-gateway provides a scalable and cost-efficient way to configure fixed source IP for Kubernetes pod egress traffic on Azure.
kube-egress-gateway components run in kubernetes clusters, either managed (Azure Kubernetes Service, AKS) or unmanaged, utilizes one or more dedicated kubernetes nodes as pod egress gateways and routes pod outbound traffic to gateway via wireguard tunnel.

Compared with existing methods, for example, creating dedicated kubernetes nodes with NAT gateway or instance level public ip address and only scheduling pods with such requirement on these nodes, kube-egress-gateway provides a more cost-efficient method as pods requiring different egress IPs can share the same gateway and can be scheduled on any regular worker nodes. 

![Kube Egress Gateway](docs/images/kube_egress_gateway.png)

## Design

* [Design doc](docs/design.md)

## Installation

TODO

## Usage

TODO

## Troubleshooting

TODO


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
