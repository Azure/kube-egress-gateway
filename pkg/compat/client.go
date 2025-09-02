// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package compat

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CompatClient is a wrapper for controller-runtime's client.Client to handle
// API changes in Go 1.25.0. It ensures type compatibility with both pre and post 1.25.0 APIs.
type CompatClient struct {
	client client.Client
}

// NewCompatClient creates a new compatibility client wrapper
func NewCompatClient(c client.Client) *CompatClient {
	return &CompatClient{client: c}
}

// Get retrieves an object from the Kubernetes API server
func (c *CompatClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return c.client.Get(ctx, key, obj, opts...)
}

// List retrieves list of objects from the Kubernetes API server
func (c *CompatClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return c.client.List(ctx, list, opts...)
}

// Create creates an object in the Kubernetes API server
func (c *CompatClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return c.client.Create(ctx, obj, opts...)
}

// Delete deletes an object from the Kubernetes API server
func (c *CompatClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return c.client.Delete(ctx, obj, opts...)
}

// Update updates an object in the Kubernetes API server
func (c *CompatClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return c.client.Update(ctx, obj, opts...)
}

// Patch patches an object in the Kubernetes API server
func (c *CompatClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return c.client.Patch(ctx, obj, patch, opts...)
}

// Apply creates or patches an object in the Kubernetes API server
func (c *CompatClient) Apply(ctx context.Context, obj runtime.ApplyConfiguration, opts ...client.ApplyOption) error {
	return c.client.Apply(ctx, obj, opts...)
}

// DeleteAllOf deletes all objects of the given type matching the given options
func (c *CompatClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return c.client.DeleteAllOf(ctx, obj, opts...)
}

// Status returns the status client
func (c *CompatClient) Status() client.SubResourceWriter {
	return c.client.Status()
}

// Scheme returns the scheme used by the client
func (c *CompatClient) Scheme() *runtime.Scheme {
	return c.client.Scheme()
}

// GroupVersionKindFor returns the GroupVersionKind for the given object
func (c *CompatClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return c.client.GroupVersionKindFor(obj)
}

// RESTMapper returns the REST mapper
func (c *CompatClient) RESTMapper() meta.RESTMapper {
	return c.client.RESTMapper()
}

// IsObjectNamespaced returns whether the object is namespaced
func (c *CompatClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return c.client.IsObjectNamespaced(obj)
}

// SubResource returns a sub resource interface
func (c *CompatClient) SubResource(subResource string) client.SubResourceClient {
	return c.client.SubResource(subResource)
}
