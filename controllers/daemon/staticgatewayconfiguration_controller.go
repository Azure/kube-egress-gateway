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

package daemon

import (
	"context"

	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &StaticGatewayConfigurationReconciler{}

// StaticGatewayConfigurationReconciler reconciles gateway node network according to a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	*azmanager.AzureManager
}

//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations,verbs=get;list;watch;
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=staticgatewayconfigurations/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the StaticGatewayConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *StaticGatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("get one StaticGatewayConfiguration")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&kubeegressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Complete(r)
}
