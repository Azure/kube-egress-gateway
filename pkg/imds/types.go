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
package imds

type InstanceMetadata struct {
	Compute *ComputeMetadata `json:"compute"`
	Network *NetworkMetadata `json:"network"`
}

type ComputeMetadata struct {
	AzEnvironment     string `json:"azEnvironment"`
	Location          string `json:"location"`
	Name              string `json:"name"`
	OSType            string `json:"osType"`
	ResourceGroupName string `json:"resourceGroupName"`
	ResourceID        string `json:"resourceId"`
	SubscriptionID    string `json:"subscriptionId"`
	Tags              string `json:"tags"`
	VMScaleSetName    string `json:"vmScaleSetName"`
}

type NetworkMetadata struct {
	Interface []NetworkInterface `json:"interface"`
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
