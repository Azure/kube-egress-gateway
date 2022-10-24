package controllers

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
	WireguardPortStart = 6000

	// Wireguard listening port range end, exclusive
	WireguardPortEnd = 7000

	// Wireguard daemon on gateway nodes listening port
	WireguardDaemonServicePort = 8080

	// nodepool name tag key in aks clusters
	AKSNodepoolTagKey = "aks-managed-poolName"

	// gateway nodepool ip prefix size tag key in aks clusters
	AKSNodepoolIPPrefixSizeTagKey = "aks-managed-gatewayIPPrefixSize"
)
