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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayVMSSProfile finds an existing gateway VMSS (virtual machine scale set).
type GatewayVMSSProfile struct {
	// Resource group of the VMSS. Must be in the same subscription.
	VMSSResourceGroup string `json:"vmssResourceGroup,omitempty"`

	// Name of the VMSS
	VMSSName string `json:"vmssName,omitempty"`

	// Public IP prefix size to be applied to this VMSS.
	PublicIpPrefixSize int32 `json:"publicIpPrefixSize,omitempty"`
}

// StaticGatewayConfigurationSpec defines the desired state of StaticGatewayConfiguration
type StaticGatewayConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the gateway nodepool to apply the gateway configuration.
	// +optional
	GatewayNodepoolName string `json:"gatewayNodepoolName,omitempty"`

	// Profile of the gateway VMSS to apply the gateway configuration.
	// +optional
	GatewayVMSSProfile `json:"gatewayVmssProfile,omitempty"`

	// BYO Resource ID of public IP prefix to be used as outbound.
	// +optional
	PublicIpPrefixId string `json:"publicIpPrefixId,omitempty"`
}

// GatewayWireguardProfile provides details about gateway side wireguard configuration.
type GatewayWireguardProfile struct {
	// Gateway IP for wireguard connection.
	WireguardServerIP string `json:"wireguardServerIP,omitempty"`

	// Listening port of the gateway side wireguard daemon.
	WireguardServerPort int32 `json:"wireguardServerPort,omitempty"`

	// Gateway side wireguard public key.
	WireguardPublicKey string `json:"wireguardPublicKey,omitempty"`

	// Reference of the secret that holds gateway side wireguard private key.
	WireguardPrivateKeySecretRef *corev1.ObjectReference `json:"wireguardPrivateKeySecretRef,omitempty"`
}

// StaticGatewayConfigurationStatus defines the observed state of StaticGatewayConfiguration
type StaticGatewayConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// State of the GatewayConfiguration.
	State string `json:"state,omitempty"`

	// Additional message (e.g. error) to explain the state.
	Message string `json:"message,omitempty"`

	// Public IP Prefix CIDR used for this gateway configuration.
	PublicIpPrefix string `json:"publicIpPrefix,omitempty"`

	// Gateway side wireguard profile.
	GatewayWireguardProfile `json:"gatewayWireguardProfile,omitempty"`

	// List of active nodes that have this gateway configuration setup ready.
	ActiveNodes []string `json:"activeNodes,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StaticGatewayConfiguration is the Schema for the staticgatewayconfigurations API
type StaticGatewayConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StaticGatewayConfigurationSpec   `json:"spec,omitempty"`
	Status StaticGatewayConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StaticGatewayConfigurationList contains a list of StaticGatewayConfiguration
type StaticGatewayConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StaticGatewayConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StaticGatewayConfiguration{}, &StaticGatewayConfigurationList{})
}
