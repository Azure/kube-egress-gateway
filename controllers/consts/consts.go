package consts

const (
	// StaticGatewayConfiguration finalizer name
	SGCFinalizerName = "static-gateway-configuration-controller.microsoft.com"

	// GatewayLBConfiguration finalizer name
	LBConfigFinalizerName = "gateway-lb-configuration-controller.microsoft.com"

	// GatewayVMConfiguration finalizer name
	VMConfigFinalizerName = "gateway-vm-configuration-controller.microsoft.com"

	// Key name in the wireugard private key secret
	WireguardSecretKeyName = "WireguardPrivateKey"

	// Wireguard listening port range start, inclusive
	WireguardPortStart int32 = 6000

	// Wireguard listening port range end, exclusive
	WireguardPortEnd int32 = 7000

	// Wireguard daemon on gateway nodes listening port
	WireguardDaemonServicePort int32 = 8080

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

	// nat table name
	NatTable = "nat"

	// environment variable name for pod namespace
	PodNamespaceEnvKey = "MY_POD_NAMESPACE"

	// environment variable name for nodeName
	NodeNameEnvKey = "MY_NODE_NAME"
)
