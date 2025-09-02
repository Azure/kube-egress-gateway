package compat

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestCompatClient tests that our compatibility layer works correctly
func TestCompatClient(t *testing.T) {
	// Create a fake client
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	cl := fake.NewClientBuilder().WithScheme(s).Build()

	// Wrap it with our compatibility layer
	compatClient := NewCompatClient(cl)

	// Make sure the wrapped client implements the client.Client interface
	var _ client.Client = compatClient

	// If we got this far, the type check passed
	t.Log("CompatClient correctly implements client.Client interface")
}

// TestObjectKey tests that our ObjectKey function works correctly
func TestObjectKey(t *testing.T) {
	key := ObjectKey("test-namespace", "test-name")

	if key.Name != "test-name" {
		t.Errorf("Expected name to be 'test-name', got '%s'", key.Name)
	}

	if key.Namespace != "test-namespace" {
		t.Errorf("Expected namespace to be 'test-namespace', got '%s'", key.Namespace)
	}
}
