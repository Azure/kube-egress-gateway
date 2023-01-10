/*
MIT License

Copyright (c) Microsoft Corporation.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/
package consts

const (
	// GatewayLBConfiguration finalizer name
	LBConfigFinalizerName = "gateway-lb-configuration-controller.microsoft.com"

	// GatewayVMConfiguration finalizer name
	VMConfigFinalizerName = "gateway-vm-configuration-controller.microsoft.com"

	// Key name in the wireugard private key secret
	WireguardSecretKeyName = "WireguardPrivateKey"

	// Key name in the wireugard private key secret
	WireguardPublicKeyName = "WireguardPublicKey"

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

const (
	CNISocketPath = "/var/run/egressgateway.sock"
)
