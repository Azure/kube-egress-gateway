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

// GatewayWireguardEndpointSpec defines the desired state of GatewayWireguardEndpoint
type GatewayWireguardEndpointSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of the node to this GatewayWireguardEndpoint belongs to.
	NodeName string `json:"nodeName,omitempty"`

	// Name of the StaticGatewayConfiguration resource this GatewayWireguardEndpoint belongs to.
	StaticGatewayConfiguration string `json:"staticGatewayConfiguration,omitempty"`

	// The private IP assigned to the gateway node.
	GWPrivateIpAddress string `json:"gwPrivateIpAddress,omitempty"`

	// Name of the secret containing wireguard keypair information.
	WireguardKeySecret string `json:"wireguardKeySecret,omitempty"`
}

// GatewayWireguardEndpointStatus defines the observed state of GatewayWireguardEndpoint
type GatewayWireguardEndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// State of the endpoint.
	State string `json:"state,omitempty"`

	// Additional message (e.g. error) to explain the state.
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GatewayWireguardEndpoint is the Schema for the gatewaywireguardendpoints API
type GatewayWireguardEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayWireguardEndpointSpec   `json:"spec,omitempty"`
	Status GatewayWireguardEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayWireguardEndpointList contains a list of GatewayWireguardEndpoint
type GatewayWireguardEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayWireguardEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayWireguardEndpoint{}, &GatewayWireguardEndpointList{})
}
