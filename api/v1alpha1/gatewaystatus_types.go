// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type GatewayConfiguration struct {
	// StaticGatewayConfiguration in <namespace>/<name> pattern
	StaticGatewayConfiguration string `json:"staticGatewayConfiguration,omitempty"`
	// Network interface name
	InterfaceName string `json:"interfaceName,omitempty"`
}

type PeerConfiguration struct {
	// PodEndpoint in <namespace>/<name> pattern
	PodEndpoint string `json:"podEndpoint,omitempty"`
	// Network interface name
	InterfaceName string `json:"interfaceName,omitempty"`
	// Public Key
	PublicKey string `json:"publicKey,omitempty"`
}

// GatewayStatusSpec defines the desired state of GatewayStatus
type GatewayStatusSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// List of ready gateway configurations
	ReadyGatewayConfigurations []GatewayConfiguration `json:"readyGatewayConfigurations,omitempty"`
	// List of ready peer configurations
	ReadyPeerConfigurations []PeerConfiguration `json:"readyPeerConfigurations,omitempty"`
}

// GatewayStatusStatus defines the observed state of GatewayStatus
type GatewayStatusStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GatewayStatus is the Schema for the gatewaystatuses API
type GatewayStatus struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayStatusSpec   `json:"spec,omitempty"`
	Status GatewayStatusStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayStatusList contains a list of GatewayStatus
type GatewayStatusList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayStatus `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayStatus{}, &GatewayStatusList{})
}
