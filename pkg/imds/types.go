// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package imds

type InstanceMetadata struct {
	Compute *ComputeMetadata `json:"compute"`
	Network *NetworkMetadata `json:"network"`
}

type ComputeMetadata struct {
	AzEnvironment     string    `json:"azEnvironment"`
	Location          string    `json:"location"`
	Name              string    `json:"name"`
	OSType            string    `json:"osType"`
	OSProfile         OSProfile `json:"osProfile"`
	ResourceGroupName string    `json:"resourceGroupName"`
	ResourceID        string    `json:"resourceId"`
	SubscriptionID    string    `json:"subscriptionId"`
	Tags              string    `json:"tags"`
	VMScaleSetName    string    `json:"vmScaleSetName"`
}

type NetworkMetadata struct {
	Interface []NetworkInterface `json:"interface"`
}

type OSProfile struct {
	ComputerName string `json:"computerName"`
}

type NetworkInterface struct {
	IPv4       IPData `json:"ipv4"`
	MacAddress string `json:"macAddress"`
}

type IPData struct {
	IPAddress []IPAddress `json:"ipAddress"`
	Subnet    []Subnet    `json:"subnet"`
}

type IPAddress struct {
	PrivateIP string `json:"privateIpAddress"`
	PublicIP  string `json:"publicIpAddress"`
}
type Subnet struct {
	Address string `json:"address"`
	Prefix  string `json:"prefix"`
}

type LoadBalancerMetadata struct {
	LoadBalancer LBData `json:"loadbalancer"`
}

type LBData struct {
	PublicIPAddresses []PublicIPMetadata `json:"publicIpAddresses"`
}

type PublicIPMetadata struct {
	FrontendIPAddress string `json:"frontendIpAddress"`
	PrivateIPAddress  string `json:"privateIpAddress"`
}
