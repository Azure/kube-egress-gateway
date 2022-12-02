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
	"k8s.io/apimachinery/pkg/runtime"
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

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-kube-egress-gateway-microsoft-com-v1alpha1-staticgatewayconfiguration,mutating=true,failurePolicy=fail,sideEffects=None,groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=create;update,versions=v1alpha1,name=mstaticgatewayconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &StaticGatewayConfiguration{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) Default() {
	staticgatewayconfigurationlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-kube-egress-gateway-microsoft-com-v1alpha1-staticgatewayconfiguration,mutating=false,failurePolicy=fail,sideEffects=None,groups=egressgateway.kubernetes.azure.com,resources=staticgatewayconfigurations,verbs=create;update,versions=v1alpha1,name=vstaticgatewayconfiguration.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &StaticGatewayConfiguration{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateCreate() error {
	staticgatewayconfigurationlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateUpdate(old runtime.Object) error {
	staticgatewayconfigurationlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *StaticGatewayConfiguration) ValidateDelete() error {
	staticgatewayconfigurationlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
