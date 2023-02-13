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
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var staticgatewayconfigurationlog = logf.Log.WithName("staticgatewayconfiguration-resource")

func (r *StaticGatewayConfiguration) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-egressgateway-kubernetes-azure-com-v1alpha1-staticgatewayconfiguration,mutating=true,failurePolicy=fail,sideEffects=None,groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=create;update,versions=v1alpha1,name=mstaticgatewayconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &StaticGatewayConfiguration{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) Default() {
	staticgatewayconfigurationlog.Info("default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-egressgateway-kubernetes-azure-com-v1alpha1-staticgatewayconfiguration,mutating=false,failurePolicy=fail,sideEffects=None,groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=create;update,versions=v1alpha1,name=vstaticgatewayconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &StaticGatewayConfiguration{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateCreate() error {
	staticgatewayconfigurationlog.Info("validate create", "name", r.Name)
	return r.validateSGC()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateUpdate(old runtime.Object) error {
	staticgatewayconfigurationlog.Info("validate update", "name", r.Name)
	return r.validateSGC()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateDelete() error {
	staticgatewayconfigurationlog.Info("validate delete", "name", r.Name)
	// no need to validate delete at this moment
	return nil
}

func (r *StaticGatewayConfiguration) validateSGC() error {
	// need to validate either GatewayNodepoolName or GatewayVMSSProfile is provided, but not both
	var allErrs field.ErrorList

	if r.Spec.GatewayNodepoolName == "" && r.vmssProfileIsEmpty() {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewaynodepoolname"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVMSSProfile: %#v", r.Spec.GatewayNodepoolName, r.Spec.GatewayVMSSProfile),
			"Either GatewayNodepoolName or GatewayVMSSProfile must be provided"))
	}

	if r.Spec.GatewayNodepoolName != "" && !r.vmssProfileIsEmpty() {
		allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewaynodepoolname"),
			fmt.Sprintf("GatewayNodepoolName: %s, GatewayVMSSProfile: %#v", r.Spec.GatewayNodepoolName, r.Spec.GatewayVMSSProfile),
			"Only one of GatewayNodepoolName and GatewayVMSSProfile should be provided"))
	}

	if !r.vmssProfileIsEmpty() {
		if r.Spec.GatewayVMSSProfile.VMSSResourceGroup == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("vmssresourcegroup"),
				r.Spec.GatewayVMSSProfile.VMSSResourceGroup,
				"Gateway vmss resource group is empty"))
		}
		if r.Spec.GatewayVMSSProfile.VMSSName == "" {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("vmssname"),
				r.Spec.GatewayVMSSProfile.VMSSName,
				"Gateway vmss name is empty"))
		}
		if r.Spec.GatewayVMSSProfile.PublicIpPrefixSize < 0 || r.Spec.GatewayVMSSProfile.PublicIpPrefixSize > 31 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec").Child("gatewayvmssprofile").Child("publicipprefixsize"),
				r.Spec.GatewayVMSSProfile.PublicIpPrefixSize,
				"Gateway vmss public ip prefix size should be between 0 and 31 inclusively"))
		}
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "egressgateway.kubernetes.azure.com", Kind: "StaticGatewayConfiguration"},
		r.Name, allErrs)
}

func (r *StaticGatewayConfiguration) vmssProfileIsEmpty() bool {
	return r.Spec.GatewayVMSSProfile.VMSSResourceGroup == "" &&
		r.Spec.GatewayVMSSProfile.VMSSName == "" &&
		r.Spec.GatewayVMSSProfile.PublicIpPrefixSize == 0
}
