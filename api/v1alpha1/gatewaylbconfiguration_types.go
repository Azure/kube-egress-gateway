// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

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

	// Profile of the gateway VM to apply the gateway configuration.
	// +optional
	GatewayVmProfile `json:"gatewayVmProfile,omitempty"`

	// Profile of the gateway pool to apply the gateway configuration.
	// This is a more generic profile that will replace both GatewayVmssProfile and GatewayVmProfile.
	// +optional
	GatewayPoolProfile `json:"gatewayPoolProfile,omitempty"`

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

	// Gateway frontend IP.
	FrontendIp string `json:"frontendIp,omitempty"`

	// Listening port of the gateway server.
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
