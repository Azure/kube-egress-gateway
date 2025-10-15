# Introduce controller-runtime compatibility layer for Go 1.25.0 migration

This PR introduces a compatibility layer to help with migrating controllers to Go 1.25.0.

## Summary

This PR provides a compatibility layer to address controller-runtime API changes in Go 1.25.0, allowing for a smoother migration path. The changes include a wrapper around the client interface and example controller implementations showing how to migrate existing controllers.

## Changes

- **Created compatibility layer in `pkg/compat`**:
  - `client.go`: Wrapper for controller-runtime's client that handles API changes
  - `MIGRATION_GUIDE.md`: Step-by-step guide for updating controllers
  - `client_test.go`: Tests for the compatibility layer

- **Added example implementations** showing the migration pattern:
  - `controllers/manager/gatewaylbconfiguration_controller_125.go`: Example showing how to update GatewayLBConfigurationReconciler
  - `controllers/manager/staticgatewayconfiguration_controller_125.go`: Example showing how to update StaticGatewayConfigurationReconciler

## Migration Status

### Controllers Updated in This PR

- None (this PR only provides examples and the compatibility layer)

### Controllers Remaining to be Updated

The following controllers need to be updated in subsequent PRs:

#### Manager Controllers

- [ ] GatewayLBConfigurationReconciler in controllers/manager
- [ ] StaticGatewayConfigurationReconciler in controllers/manager
- [ ] GatewayVMConfigurationReconciler in controllers/manager

#### Daemon Controllers

- [ ] StaticGatewayConfigurationReconciler in controllers/daemon
- [ ] PodEndpointReconciler in controllers/daemon

## Migration Checklist

For each controller:

1. Add `CompatClient` field to the reconciler struct
2. Initialize the client in the `SetupWithManager` function
3. Replace all direct client operations with compatibility client operations
4. Update test cases to use the compatibility client

## Implementation Approach

The compatibility layer provides:

1. A `CompatClient` type that wraps the controller-runtime client
2. Methods compatible with Go 1.25.0 that match the original client interface
3. Guidance for consistent controller updates

## Testing Instructions

To test a controller update with the compatibility layer:

### Unit Testing

1. Update the controller to use the compatibility client
2. Run the unit tests for the controller:

   ```bash
   go test -v ./controllers/manager/...
   ```

### Integration Testing

1. Create an integration test that specifically tests the compatibility layer:

   ```go
   func TestControllerWithCompat(t *testing.T) {
       // Create a fake client
       cl := fake.NewClientBuilder().WithObjects(testObj).Build()
       
       // Create the reconciler with the compatibility client
       r := &MyReconciler{
           Client:       cl,
           CompatClient: compat.NewCompatClient(cl),
       }
       
       // Test reconciliation
       // ...
   }
   ```

### End-to-End Testing

1. Build the Docker images:

   ```bash
   make docker-build
   ```

2. Deploy to a test cluster and verify functionality

### Debugging Tips

- Add logging to track client operations
- Compare behavior with and without the compatibility layer
- Use a debugger to step through code execution

## Next Steps

After this PR is merged:

1. Update each controller individually using the compatibility layer
2. Test each controller update thoroughly
3. Address other Go 1.25.0 compatibility issues

## Related PRs

- #XXXX (Initial Go 1.25.0 update)
