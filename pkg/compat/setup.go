// This file contains utility functions to help with controller-runtime compatibility

package compat

import (
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupControllerWithManager is a helper to set up a controller with manager
// It wraps the controller's client with our compatibility layer
func SetupControllerWithManager(
	mgr manager.Manager,
	setupFunc func(client.Client, *runtime.Scheme, record.EventRecorder, logr.Logger) error) error {

	compatClient := NewCompatClient(mgr.GetClient())
	scheme := mgr.GetScheme()
	recorder := mgr.GetEventRecorderFor("compat-controller")
	logger := ctrl.Log.WithName("compat")

	return setupFunc(compatClient, scheme, recorder, logger)
}

// WrapReconcilerClient wraps an existing reconciler's client field with our compatibility layer
func WrapReconcilerClient(reconciler interface{}) {
	// Use reflection to wrap the client field if it exists
	// This is a simplified example - in practice, you'd need to use reflection

	// For now, this is just a placeholder. In real code, you'd inspect the reconciler
	// and replace its Client field with a wrapped version using reflection.
}
