// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodWireguardEndpointSpec defines the desired state of PodWireguardEndpoint
type PodWireguardEndpointSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of StaticGatewayConfiguration the pod uses.
	StaticGatewayConfiguration string `json:"staticGatewayConfiguration,omitempty"`

	// IPv4 address assigned to the pod.
	PodIpAddress string `json:"podIpAddress,omitempty"`

	// wireguard public key on pod side.
	PodWireguardPublicKey string `json:"podWireguardPublicKey,omitempty"`
}

// PodWireguardEndpointStatus defines the observed state of PodWireguardEndpoint
type PodWireguardEndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PodWireguardEndpoint is the Schema for the podwireguardendpoints API
type PodWireguardEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodWireguardEndpointSpec   `json:"spec,omitempty"`
	Status PodWireguardEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodWireguardEndpointList contains a list of PodWireguardEndpoint
type PodWireguardEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodWireguardEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodWireguardEndpoint{}, &PodWireguardEndpointList{})
}
