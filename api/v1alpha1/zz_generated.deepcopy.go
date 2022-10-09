//go:build !ignore_autogenerated
// +build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayLBConfiguration) DeepCopyInto(out *GatewayLBConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayLBConfiguration.
func (in *GatewayLBConfiguration) DeepCopy() *GatewayLBConfiguration {
	if in == nil {
		return nil
	}
	out := new(GatewayLBConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayLBConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayLBConfigurationList) DeepCopyInto(out *GatewayLBConfigurationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GatewayLBConfiguration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayLBConfigurationList.
func (in *GatewayLBConfigurationList) DeepCopy() *GatewayLBConfigurationList {
	if in == nil {
		return nil
	}
	out := new(GatewayLBConfigurationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayLBConfigurationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayLBConfigurationSpec) DeepCopyInto(out *GatewayLBConfigurationSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayLBConfigurationSpec.
func (in *GatewayLBConfigurationSpec) DeepCopy() *GatewayLBConfigurationSpec {
	if in == nil {
		return nil
	}
	out := new(GatewayLBConfigurationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayLBConfigurationStatus) DeepCopyInto(out *GatewayLBConfigurationStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayLBConfigurationStatus.
func (in *GatewayLBConfigurationStatus) DeepCopy() *GatewayLBConfigurationStatus {
	if in == nil {
		return nil
	}
	out := new(GatewayLBConfigurationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayVMConfiguration) DeepCopyInto(out *GatewayVMConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayVMConfiguration.
func (in *GatewayVMConfiguration) DeepCopy() *GatewayVMConfiguration {
	if in == nil {
		return nil
	}
	out := new(GatewayVMConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayVMConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayVMConfigurationList) DeepCopyInto(out *GatewayVMConfigurationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GatewayVMConfiguration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayVMConfigurationList.
func (in *GatewayVMConfigurationList) DeepCopy() *GatewayVMConfigurationList {
	if in == nil {
		return nil
	}
	out := new(GatewayVMConfigurationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayVMConfigurationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayVMConfigurationSpec) DeepCopyInto(out *GatewayVMConfigurationSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayVMConfigurationSpec.
func (in *GatewayVMConfigurationSpec) DeepCopy() *GatewayVMConfigurationSpec {
	if in == nil {
		return nil
	}
	out := new(GatewayVMConfigurationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayVMConfigurationStatus) DeepCopyInto(out *GatewayVMConfigurationStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayVMConfigurationStatus.
func (in *GatewayVMConfigurationStatus) DeepCopy() *GatewayVMConfigurationStatus {
	if in == nil {
		return nil
	}
	out := new(GatewayVMConfigurationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayVMSSProfile) DeepCopyInto(out *GatewayVMSSProfile) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayVMSSProfile.
func (in *GatewayVMSSProfile) DeepCopy() *GatewayVMSSProfile {
	if in == nil {
		return nil
	}
	out := new(GatewayVMSSProfile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayWireguardEndpoint) DeepCopyInto(out *GatewayWireguardEndpoint) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayWireguardEndpoint.
func (in *GatewayWireguardEndpoint) DeepCopy() *GatewayWireguardEndpoint {
	if in == nil {
		return nil
	}
	out := new(GatewayWireguardEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayWireguardEndpoint) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayWireguardEndpointList) DeepCopyInto(out *GatewayWireguardEndpointList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]GatewayWireguardEndpoint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayWireguardEndpointList.
func (in *GatewayWireguardEndpointList) DeepCopy() *GatewayWireguardEndpointList {
	if in == nil {
		return nil
	}
	out := new(GatewayWireguardEndpointList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatewayWireguardEndpointList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayWireguardEndpointSpec) DeepCopyInto(out *GatewayWireguardEndpointSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayWireguardEndpointSpec.
func (in *GatewayWireguardEndpointSpec) DeepCopy() *GatewayWireguardEndpointSpec {
	if in == nil {
		return nil
	}
	out := new(GatewayWireguardEndpointSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayWireguardEndpointStatus) DeepCopyInto(out *GatewayWireguardEndpointStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayWireguardEndpointStatus.
func (in *GatewayWireguardEndpointStatus) DeepCopy() *GatewayWireguardEndpointStatus {
	if in == nil {
		return nil
	}
	out := new(GatewayWireguardEndpointStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatewayWireguardProfile) DeepCopyInto(out *GatewayWireguardProfile) {
	*out = *in
	if in.WireguardPrivateKeySecretRef != nil {
		in, out := &in.WireguardPrivateKeySecretRef, &out.WireguardPrivateKeySecretRef
		*out = new(v1.ObjectReference)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatewayWireguardProfile.
func (in *GatewayWireguardProfile) DeepCopy() *GatewayWireguardProfile {
	if in == nil {
		return nil
	}
	out := new(GatewayWireguardProfile)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodWireguardEndpoint) DeepCopyInto(out *PodWireguardEndpoint) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodWireguardEndpoint.
func (in *PodWireguardEndpoint) DeepCopy() *PodWireguardEndpoint {
	if in == nil {
		return nil
	}
	out := new(PodWireguardEndpoint)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodWireguardEndpoint) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodWireguardEndpointList) DeepCopyInto(out *PodWireguardEndpointList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PodWireguardEndpoint, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodWireguardEndpointList.
func (in *PodWireguardEndpointList) DeepCopy() *PodWireguardEndpointList {
	if in == nil {
		return nil
	}
	out := new(PodWireguardEndpointList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PodWireguardEndpointList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodWireguardEndpointSpec) DeepCopyInto(out *PodWireguardEndpointSpec) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodWireguardEndpointSpec.
func (in *PodWireguardEndpointSpec) DeepCopy() *PodWireguardEndpointSpec {
	if in == nil {
		return nil
	}
	out := new(PodWireguardEndpointSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PodWireguardEndpointStatus) DeepCopyInto(out *PodWireguardEndpointStatus) {
	*out = *in
	if in.ActiveNodes != nil {
		in, out := &in.ActiveNodes, &out.ActiveNodes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PodWireguardEndpointStatus.
func (in *PodWireguardEndpointStatus) DeepCopy() *PodWireguardEndpointStatus {
	if in == nil {
		return nil
	}
	out := new(PodWireguardEndpointStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StaticGatewayConfiguration) DeepCopyInto(out *StaticGatewayConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StaticGatewayConfiguration.
func (in *StaticGatewayConfiguration) DeepCopy() *StaticGatewayConfiguration {
	if in == nil {
		return nil
	}
	out := new(StaticGatewayConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StaticGatewayConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StaticGatewayConfigurationList) DeepCopyInto(out *StaticGatewayConfigurationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]StaticGatewayConfiguration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StaticGatewayConfigurationList.
func (in *StaticGatewayConfigurationList) DeepCopy() *StaticGatewayConfigurationList {
	if in == nil {
		return nil
	}
	out := new(StaticGatewayConfigurationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *StaticGatewayConfigurationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StaticGatewayConfigurationSpec) DeepCopyInto(out *StaticGatewayConfigurationSpec) {
	*out = *in
	out.GatewayVMSSProfile = in.GatewayVMSSProfile
	if in.ExceptionCIDRs != nil {
		in, out := &in.ExceptionCIDRs, &out.ExceptionCIDRs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StaticGatewayConfigurationSpec.
func (in *StaticGatewayConfigurationSpec) DeepCopy() *StaticGatewayConfigurationSpec {
	if in == nil {
		return nil
	}
	out := new(StaticGatewayConfigurationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StaticGatewayConfigurationStatus) DeepCopyInto(out *StaticGatewayConfigurationStatus) {
	*out = *in
	in.GatewayWireguardProfile.DeepCopyInto(&out.GatewayWireguardProfile)
	if in.ActiveNodes != nil {
		in, out := &in.ActiveNodes, &out.ActiveNodes
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StaticGatewayConfigurationStatus.
func (in *StaticGatewayConfigurationStatus) DeepCopy() *StaticGatewayConfigurationStatus {
	if in == nil {
		return nil
	}
	out := new(StaticGatewayConfigurationStatus)
	in.DeepCopyInto(out)
	return out
}
