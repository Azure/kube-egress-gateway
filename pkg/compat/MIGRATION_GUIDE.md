# Controller Migration Guide for Go 1.25.0

This document provides step-by-step instructions for updating controllers to use the compatibility layer for Go 1.25.0.

## Background

Go 1.25.0 introduces type parameter changes that affect how the controller-runtime package works with Kubernetes resources. The compatibility layer in `pkg/compat` helps bridge these differences without requiring major code changes to existing controllers.

## Controller Migration Status

### Controllers Already Updated
- ✅ GatewayLBConfigurationReconciler in controllers/manager

### Controllers Still to be Updated
- ⬜️ StaticGatewayConfigurationReconciler in controllers/manager
- ⬜️ GatewayVMConfigurationReconciler in controllers/manager
- ⬜️ StaticGatewayConfigurationReconciler in controllers/daemon
- ⬜️ PodEndpointReconciler in controllers/daemon

## Migration Steps for Each Controller

Follow these steps to update each remaining controller:

### Step 1: Add the CompatClient Field

Add a `CompatClient` field to your controller struct:

```go
import "github.com/Azure/kube-egress-gateway/pkg/compat"

type MyReconciler struct {
    client.Client
    CompatClient *compat.CompatClient  // Add this line
    // other fields...
}
```

### Step 2: Initialize the CompatClient

Update your controller's `SetupWithManager` function to initialize the compatibility client:

```go
func (r *MyReconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Initialize the compatibility client for Go 1.25.0
    r.CompatClient = compat.NewCompatClient(mgr.GetClient())
    
    return ctrl.NewControllerManagedBy(mgr).
        For(&myv1.MyResource{}).
        Complete(r)
}
```

### Step 3: Update Client Operations

Replace all direct client operations with compatibility client operations:

#### Get Operations
```go
// Before
if err := r.Get(ctx, req.NamespacedName, resource); err != nil { ... }

// After
if err := r.CompatClient.Get(ctx, req.NamespacedName, resource); err != nil { ... }
```

#### Status Updates
```go
// Before
if err := r.Status().Update(ctx, resource); err != nil { ... }

// After
if err := r.CompatClient.Status().Update(ctx, resource); err != nil { ... }
```

#### Other Operations
Similarly update all Create, Update, Delete, List, and Patch operations.

## Testing After Migration

After updating a controller, test it thoroughly:

1. **Unit Tests**: Run the unit tests for the updated controller:
   ```bash
   go test -v ./controllers/manager/...
   ```

2. **Integration Tests**: If you have integration tests, verify they pass:
   ```bash
   make test-integration
   ```

3. **End-to-End Testing**: Build and deploy to test in a cluster:
   ```bash
   make docker-build
   make install
   make deploy
   ```

## Common Issues and Troubleshooting

1. **Missed Client Operations**: Search thoroughly for all client operations (`Get`, `List`, `Create`, `Update`, `Delete`, `Status().Update`, etc.)

2. **Type Errors**: Watch for errors related to type parameters which indicate you missed a client operation

3. **Helper Functions**: If your controller has helper functions that use the client, update those too:
   ```go
   // Before
   func helperFunc(cl client.Client) { ... }
   
   // After - Option 1: Update parameter
   func helperFunc(cl *compat.CompatClient) { ... }
   
   // After - Option 2: Create compat client inside
   func helperFunc(cl client.Client) {
       compatCl := compat.NewCompatClient(cl)
       // Use compatCl instead of cl
   }
   ```

## Need Help?

If you encounter issues during migration, contact the maintainers or refer to the Go 1.25.0 compatibility discussions in our team meetings.
