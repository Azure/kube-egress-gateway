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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayLBConfigurationSpec defines the desired state of GatewayLBConfiguration
type GatewayLBConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the gateway nodepool to apply the gateway configuration.
	// +optional
	GatewayNodepoolName string `json:"gatewayNodepoolName,omitempty"`

	// Profile of the gateway VMSS to apply the gateway configuration.
	// +optional
	GatewayVmssProfile `json:"gatewayVmssProfile,omitempty"`

	// Whether to provision public IP prefixes for outbound.
	//+kubebuilder:default=true
	ProvisionPublicIps bool `json:"provisionPublicIps"`

	// BYO Resource ID of public IP prefix to be used as outbound.
	// +optional
	PublicIpPrefixId string `json:"publicIpPrefixId,omitempty"`
}

// GatewayLBConfigurationStatus defines the observed state of GatewayLBConfiguration
type GatewayLBConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Gateway IP for wireguard connection.
	FrontendIp string `json:"frontendIp,omitempty"`

	// Listening port of the gateway side wireguard daemon.
	ServerPort int32 `json:"serverPort,omitempty"`

	// Egress IP Prefix CIDR used for this gateway configuration.
	EgressIpPrefix string `json:"egressIpPrefix,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GatewayLBConfiguration is the Schema for the gatewaylbconfigurations API
type GatewayLBConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayLBConfigurationSpec    `json:"spec,omitempty"`
	Status *GatewayLBConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayLBConfigurationList contains a list of GatewayLBConfiguration
type GatewayLBConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayLBConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayLBConfiguration{}, &GatewayLBConfigurationList{})
}
