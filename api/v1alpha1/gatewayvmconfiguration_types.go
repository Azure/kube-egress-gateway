// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// GatewayVMConfigurationSpec defines the desired state of GatewayVMConfiguration
type GatewayVMConfigurationSpec struct {
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

// GatewayVMConfigurationStatus defines the observed state of GatewayVMConfiguration
type GatewayVMConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// The egress source IP for traffic using this configuration.
	EgressIpPrefix string `json:"egressIpPrefix,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GatewayVMConfiguration is the Schema for the gatewayvmconfigurations API
type GatewayVMConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayVMConfigurationSpec    `json:"spec,omitempty"`
	Status *GatewayVMConfigurationStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayVMConfigurationList contains a list of GatewayVMConfiguration
type GatewayVMConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayVMConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayVMConfiguration{}, &GatewayVMConfigurationList{})
}
