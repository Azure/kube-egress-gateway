// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PodEndpointSpec defines the desired state of PodEndpoint
type PodEndpointSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Name of StaticGatewayConfiguration the pod uses.
	StaticGatewayConfiguration string `json:"staticGatewayConfiguration,omitempty"`

	// IPv4 address assigned to the pod.
	PodIpAddress string `json:"podIpAddress,omitempty"`

	// public key on pod side.
	PodPublicKey string `json:"podPublicKey,omitempty"`
}

// PodEndpointStatus defines the observed state of PodEndpoint
type PodEndpointStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PodEndpoint is the Schema for the podendpoints API
type PodEndpoint struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodEndpointSpec   `json:"spec,omitempty"`
	Status PodEndpointStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PodEndpointList contains a list of PodEndpoint
type PodEndpointList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodEndpoint `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodEndpoint{}, &PodEndpointList{})
}
