// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package consts

const (
	// GatewayLBConfiguration finalizer name
	LBConfigFinalizerName = "gateway-lb-configuration-controller.microsoft.com"

	// GatewayVMConfiguration finalizer name
	VMConfigFinalizerName = "gateway-vm-configuration-controller.microsoft.com"

	// Default gateway LoadBalancer name
	DefaultGatewayLBName = "kubeegressgateway-ilb"

	// Key name in the wireugard private key secret
	WireguardSecretKeyName = "WireguardPrivateKey"

	// Key name in the wireugard private key secret
	WireguardPublicKeyName = "WireguardPublicKey"

	// Wireguard listening port range start, inclusive
	WireguardPortStart int32 = 6000

	// Wireguard listening port range end, exclusive
	WireguardPortEnd int32 = 7000

	// Gateway lb health probe path
	GatewayHealthProbeEndpoint = "/gw/"

	// Gateway lb health probe port
	WireguardDaemonServicePort int32 = 8082

	// nodepool name tag key in aks clusters
	AKSNodepoolTagKey = "aks-managed-poolName"

	// gateway nodepool ip prefix size tag key in aks clusters
	AKSNodepoolIPPrefixSizeTagKey = "aks-managed-gatewayIPPrefixSize"
)

const (
	// wireguard link name in gateway namespace
	WireguardLinkName = "wg0"

	// host link name in gateway namespace
	HostLinkName = "host0"

	// gateway IP
	GatewayIP = "fe80::1/64"

	// post routing chain name
	PostRoutingChain = "POSTROUTING"

	// pre routing chain name
	PreRoutingChain = "PREROUTING"

	// output chain name
	OutputChain = "OUTPUT"

	// nat table name
	NatTable = "nat"

	// mangle table name
	MangleTable = "mangle"

	// environment variable name for pod namespace
	PodNamespaceEnvKey = "MY_POD_NAMESPACE"

	// environment variable name for nodeName
	NodeNameEnvKey = "MY_NODE_NAME"

	// iptables rule comment
	IPTablesRuleComment = "no SNAT for traffic from netns "

	// mark for traffic from eth0 in pod namespace - 0x2222
	Eth0Mark int = 8738
)

const (
	CNISocketPath = "/var/run/egressgateway.sock"

	CNIConfDir = "/etc/cni/net.d"

	CNIGatewayAnnotationKey = "kubernetes.azure.com/static-gateway-configuration"
)

const (
	KubeEgressCNIName     = "kube-egress-cni"
	KubeEgressIPAMCNIName = "kube-egress-cni-ipam"
)
