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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockController simulates a controller that uses our compatibility client
type MockController struct {
	Client       client.Client
	CompatClient *CompatClient
}

// TestSetupWithManager tests that our compatibility layer can be properly initialized in a controller
func TestSetupWithManager(t *testing.T) {
	// Create a scheme
	scheme := runtime.NewScheme()
	scheme.AddKnownTypes(metav1.SchemeGroupVersion, &TestObject{}, &TestObjectList{})

	// Create a fake client
	cl := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create a mock controller
	mockController := &MockController{
		Client: cl,
	}

	// Initialize the compatibility client (simulating what happens in SetupWithManager)
	mockController.CompatClient = NewCompatClient(mockController.Client)

	// Verify the CompatClient was properly initialized
	require.NotNil(t, mockController.CompatClient)
}

// TestControllerOperations tests that controller operations using the compatibility client work properly
func TestControllerOperations(t *testing.T) {
	// Create a scheme
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

	// Create a mock controller with the compatibility client
	mockController := &MockController{
		Client:       cl,
		CompatClient: NewCompatClient(cl),
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
