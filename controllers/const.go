package controllers

const (
	// StaticGatewayConfiguration finalizer name
	SGCFinalizerName = "static-gateway-configuration-controller.microsoft.com"

	// Key name in the wireugard private key secret
	WireguardSecretKeyName = "WireguardPrivateKey"

	// Wireguard listening port range start, inclusive
	WireguardPortStart = 6000

	// Wireguard listening port range end, exclusive
	WireguardPortEnd = 7000
)
