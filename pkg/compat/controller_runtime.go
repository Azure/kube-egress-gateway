// This file contains patches to make controller-runtime code compatible with Go 1.25.0
// It provides wrapper types and functions for compatibility

package compat

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompatClient wraps controller-runtime Client to ensure compatibility with Go 1.25.0
type CompatClient struct {
	client.Client
}

// Get retrieves an object with compatibility for Go 1.25.0
func (c *CompatClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.Client.Get(ctx, key, obj, opts...)
}

// List retrieves list of objects with compatibility for Go 1.25.0
func (c *CompatClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.Client.List(ctx, list, opts...)
}

// Create saves a new object with compatibility for Go 1.25.0
func (c *CompatClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.Client.Create(ctx, obj, opts...)
}

// Delete deletes an object with compatibility for Go 1.25.0
func (c *CompatClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.Client.Delete(ctx, obj, opts...)
}

// Update updates an object with compatibility for Go 1.25.0
func (c *CompatClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.Client.Update(ctx, obj, opts...)
}

// Patch patches an object with compatibility for Go 1.25.0
func (c *CompatClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.Client.Patch(ctx, obj, patch, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the given options with compatibility for Go 1.25.0
func (c *CompatClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.Client.DeleteAllOf(ctx, obj, opts...)
}

// NewCompatClient creates a new compatibility client wrapper
func NewCompatClient(c client.Client) *CompatClient {
	return &CompatClient{Client: c}
}

// NamespacedName is a compatibility wrapper for types.NamespacedName
type NamespacedName struct {
	types.NamespacedName
}

// NewNamespacedName creates a new compatibility wrapper for NamespacedName
func NewNamespacedName(name, namespace string) NamespacedName {
	return NamespacedName{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// SchemeBuilder is a compatibility wrapper for runtime.SchemeBuilder
type SchemeBuilder struct {
	runtime.SchemeBuilder
}

// NewSchemeBuilder creates a new compatibility wrapper for SchemeBuilder
func NewSchemeBuilder(funcs ...func(*runtime.Scheme) error) *SchemeBuilder {
	return &SchemeBuilder{
		SchemeBuilder: runtime.SchemeBuilder(funcs),
	}
}

// AddToScheme adds all registered types to the scheme
func (s *SchemeBuilder) AddToScheme(scheme *runtime.Scheme) error {
	return s.SchemeBuilder.AddToScheme(scheme)
}
