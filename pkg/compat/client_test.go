// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package compat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestObject is a simple test object for our tests
type TestObject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TestObjectSpec `json:"spec,omitempty"`
}

// TestObjectSpec contains test data
type TestObjectSpec struct {
	Value string `json:"value"`
}

// TestObjectList is a list of TestObjects
type TestObjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TestObject `json:"items"`
}

func (t *TestObject) GetObjectKind() schema.ObjectKind {
	return &t.TypeMeta
}

func (t *TestObject) DeepCopyObject() runtime.Object {
	copy := *t
	return &copy
}

func (t *TestObjectList) GetObjectKind() schema.ObjectKind {
	return &t.TypeMeta
}

func (t *TestObjectList) DeepCopyObject() runtime.Object {
	copy := *t
	return &copy
}

func TestCompatClientGet(t *testing.T) {
	// Create a scheme and register our test types
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(metav1.SchemeGroupVersion, &TestObject{}, &TestObjectList{})

	// Create a test object
	obj := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-obj",
			Namespace: "default",
		},
		Spec: TestObjectSpec{
			Value: "test-value",
		},
	}

	// Create a fake client with the test object
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()

	// Create our compatibility client
	compatCl := NewCompatClient(cl)

	// Try to get the object
	result := &TestObject{}
	err := compatCl.Get(context.Background(), types.NamespacedName{Name: "test-obj", Namespace: "default"}, result)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, "test-obj", result.Name)
	assert.Equal(t, "test-value", result.Spec.Value)
}

func TestCompatClientStatus(t *testing.T) {
	// Create a scheme and register our test types
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(metav1.SchemeGroupVersion, &TestObject{}, &TestObjectList{})

	// Create a test object
	obj := &TestObject{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-obj",
			Namespace: "default",
		},
		Spec: TestObjectSpec{
			Value: "test-value",
		},
	}

	// Create a fake client with the test object
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()

	// Create our compatibility client
	compatCl := NewCompatClient(cl)

	// Get the object
	result := &TestObject{}
	err := compatCl.Get(context.Background(), types.NamespacedName{Name: "test-obj", Namespace: "default"}, result)
	require.NoError(t, err)

	// Update it
	result.Spec.Value = "updated-value"
	err = compatCl.Update(context.Background(), result)
	require.NoError(t, err)

	// Get it again to verify the update
	updated := &TestObject{}
	err = compatCl.Get(context.Background(), types.NamespacedName{Name: "test-obj", Namespace: "default"}, updated)
	require.NoError(t, err)
	assert.Equal(t, "updated-value", updated.Spec.Value)
}
