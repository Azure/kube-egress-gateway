// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package examples

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/Azure/kube-egress-gateway/pkg/compat"
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

// MockController simulates a controller that uses our compatibility client
type MockController struct {
	Client       client.Client
	CompatClient *compat.CompatClient
	Scheme       *runtime.Scheme
}

// TestControllerWithCompatClient tests that controller operations using the compatibility client work properly
func TestControllerWithCompatClient(t *testing.T) {
	// Create a scheme
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(metav1.SchemeGroupVersion, &TestObject{}, &TestObjectList{})

	// Create a test object
	obj := &TestObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "TestObject",
		},
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

	// Create a mock controller with the compatibility client
	mockController := &MockController{
		Client:       cl,
		CompatClient: compat.NewCompatClient(cl),
		Scheme:       scheme,
	}

	// Test Get operation
	result := &TestObject{}
	err := mockController.CompatClient.Get(context.Background(), types.NamespacedName{Name: "test-obj", Namespace: "default"}, result)
	require.NoError(t, err)
	assert.Equal(t, "test-value", result.Spec.Value)

	// Test List operation
	listResult := &TestObjectList{}
	err = mockController.CompatClient.List(context.Background(), listResult)
	require.NoError(t, err)
	assert.Equal(t, 1, len(listResult.Items))
	assert.Equal(t, "test-obj", listResult.Items[0].Name)

	// Test Create operation
	newObj := &TestObject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "TestObject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "new-obj",
			Namespace: "default",
		},
		Spec: TestObjectSpec{
			Value: "new-value",
		},
	}
	err = mockController.CompatClient.Create(context.Background(), newObj)
	require.NoError(t, err)

	// Verify the object was created
	createdObj := &TestObject{}
	err = mockController.CompatClient.Get(context.Background(), types.NamespacedName{Name: "new-obj", Namespace: "default"}, createdObj)
	require.NoError(t, err)
	assert.Equal(t, "new-value", createdObj.Spec.Value)

	// Test Update operation
	createdObj.Spec.Value = "updated-value"
	err = mockController.CompatClient.Update(context.Background(), createdObj)
	require.NoError(t, err)

	// Verify the object was updated
	updatedObj := &TestObject{}
	err = mockController.CompatClient.Get(context.Background(), types.NamespacedName{Name: "new-obj", Namespace: "default"}, updatedObj)
	require.NoError(t, err)
	assert.Equal(t, "updated-value", updatedObj.Spec.Value)

	// Test Delete operation
	err = mockController.CompatClient.Delete(context.Background(), updatedObj)
	require.NoError(t, err)

	// Verify the object was deleted
	deletedObj := &TestObject{}
	err = mockController.CompatClient.Get(context.Background(), types.NamespacedName{Name: "new-obj", Namespace: "default"}, deletedObj)
	assert.Error(t, err) // Should get an error since the object was deleted
}

// TestUnstructuredObjects tests compatibility with unstructured objects
func TestUnstructuredObjects(t *testing.T) {
	// Create a scheme
	scheme := runtime.NewScheme()

	// Create a fake client
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create a mock controller with the compatibility client
	mockController := &MockController{
		Client:       cl,
		CompatClient: compat.NewCompatClient(cl),
		Scheme:       scheme,
	}

	// Create an unstructured object
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})
	obj.SetName("test-configmap")
	obj.SetNamespace("default")
	obj.Object["data"] = map[string]interface{}{
		"key": "value",
	}

	// Create the object
	err := mockController.CompatClient.Create(context.Background(), obj)
	require.NoError(t, err)

	// Retrieve the object
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})

	err = mockController.CompatClient.Get(context.Background(), types.NamespacedName{Name: "test-configmap", Namespace: "default"}, result)
	require.NoError(t, err)

	data, found, err := unstructured.NestedStringMap(result.Object, "data")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "value", data["key"])
}
