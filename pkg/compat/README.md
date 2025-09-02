# Go 1.25.0 Compatibility Layer

This package provides compatibility wrappers for controller-runtime and Kubernetes API types
to work with Go 1.25.0. 

## Background

Go 1.25.0 introduced changes that affect how controller-runtime and some Kubernetes packages work,
particularly around API types and client interfaces. This compatibility layer ensures our code
works correctly with these changes.

## Usage

### For controller-runtime Client

```go
import "github.com/Azure/kube-egress-gateway/pkg/compat"

// Wrap an existing client
originalClient := mgr.GetClient()
compatClient := compat.NewCompatClient(originalClient)

// Use the compatClient instead of the original client
err := compatClient.Get(ctx, client.ObjectKey{Name: "foo", Namespace: "bar"}, &myObj)
```

### For types.NamespacedName

```go
import "github.com/Azure/kube-egress-gateway/pkg/compat"

// Create a namespaced name
name := compat.NewNamespacedName("my-object", "my-namespace")
```

### For SchemeBuilder

```go
import "github.com/Azure/kube-egress-gateway/pkg/compat"

// Create a scheme builder
schemeBuilder := compat.NewSchemeBuilder(mySchemeFunc)
err := schemeBuilder.AddToScheme(scheme)
```

## Helper Functions

The package also provides helper functions like `ObjectKey`, `GetObject`, `ListObjects`, etc.
to make working with controller-runtime easier.

## Future Work

This compatibility layer is temporary and should be removed once all our code has been properly
updated for Go 1.25.0.
