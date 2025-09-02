// This file contains utility functions to help with Kubernetes API types compatibility

package compat

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectKey wraps types.NamespacedName to ensure compatibility
func ObjectKey(namespace, name string) client.ObjectKey {
	return client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
}

// ToUnstructured converts a runtime.Object to an Unstructured
func ToUnstructured(obj runtime.Object, scheme *runtime.Scheme) (*unstructured.Unstructured, error) {
	u := &unstructured.Unstructured{}
	if err := scheme.Convert(obj, u, nil); err != nil {
		return nil, err
	}
	return u, nil
}

// FromUnstructured converts an Unstructured to a typed object
func FromUnstructured(u *unstructured.Unstructured, obj runtime.Object, scheme *runtime.Scheme) error {
	return scheme.Convert(u, obj, nil)
}

// GetObject is a helper to get an object using the compatibility client
func GetObject(ctx context.Context, c client.Client, key client.ObjectKey, obj client.Object) error {
	compatClient := NewCompatClient(c)
	return compatClient.Get(ctx, key, obj)
}

// ListObjects is a helper to list objects using the compatibility client
func ListObjects(ctx context.Context, c client.Client, list client.ObjectList, opts ...client.ListOption) error {
	compatClient := NewCompatClient(c)
	return compatClient.List(ctx, list, opts...)
}

// CreateObject is a helper to create an object using the compatibility client
func CreateObject(ctx context.Context, c client.Client, obj client.Object, opts ...client.CreateOption) error {
	compatClient := NewCompatClient(c)
	return compatClient.Create(ctx, obj, opts...)
}

// UpdateObject is a helper to update an object using the compatibility client
func UpdateObject(ctx context.Context, c client.Client, obj client.Object, opts ...client.UpdateOption) error {
	compatClient := NewCompatClient(c)
	return compatClient.Update(ctx, obj, opts...)
}

// DeleteObject is a helper to delete an object using the compatibility client
func DeleteObject(ctx context.Context, c client.Client, obj client.Object, opts ...client.DeleteOption) error {
	compatClient := NewCompatClient(c)
	return compatClient.Delete(ctx, obj, opts...)
}

// PatchObject is a helper to patch an object using the compatibility client
func PatchObject(ctx context.Context, c client.Client, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	compatClient := NewCompatClient(c)
	return compatClient.Patch(ctx, obj, patch, opts...)
}
