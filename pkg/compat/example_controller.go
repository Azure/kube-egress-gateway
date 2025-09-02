// Example of how to update a controller to use the compatibility layer

package example

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/kube-egress-gateway/pkg/compat"
)

// ExampleReconciler demonstrates how to update a controller to use the compatibility layer
type ExampleReconciler struct {
	client.Client // This will be wrapped by our compatibility layer
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *ExampleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Wrap the client with our compatibility layer
	r.Client = compat.NewCompatClient(mgr.GetClient())
	
	return ctrl.NewControllerManagedBy(mgr).
		// Add your controller configuration here
		Complete(r)
}

// Reconcile shows how to use the compatibility layer in your reconcile function
func (r *ExampleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling object with compatibility layer")
	
	// Example of getting an object using the compatibility layer directly
	obj := &SomeObject{}
	if err := r.Get(ctx, client.ObjectKey{Name: req.Name, Namespace: req.Namespace}, obj); err != nil {
		// Handle error
		return ctrl.Result{}, err
	}
	
	// Example of using helper functions from the compatibility layer
	anotherObj := &AnotherObject{}
	if err := compat.GetObject(ctx, r.Client, compat.ObjectKey(req.Namespace, "some-name"), anotherObj); err != nil {
		// Handle error
		return ctrl.Result{}, err
	}
	
	// Rest of your reconcile logic
	
	return ctrl.Result{}, nil
}

// These are placeholder types for the example
type SomeObject struct {
	client.Object
}

type AnotherObject struct {
	client.Object
}
