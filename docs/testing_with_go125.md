# Testing with Go 1.25.0

## Introduction

This document explains how to run tests for the kube-egress-gateway project with Go 1.25.0, including how to handle integration tests.

## Compatibility Layer
All controller tests must use the compatibility layer to work with Go 1.25.0. Each controller has a helper function that initializes the `CompatClient` properly:

- `createVMConfigReconciler` - For GatewayVMConfigurationReconciler
- `createLBConfigReconciler` - For GatewayLBConfigurationReconciler
- `createStaticGWConfigReconciler` - For StaticGatewayConfigurationReconciler (manager)

## Running Tests

### Unit Tests Only
To run only unit tests without integration tests:

```bash
go test -tags=skip_integration ./...
```

### Integration Tests
To run integration tests, you need etcd installed. First, install kubebuilder:

```bash
# Install kubebuilder
curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.12.0/kubebuilder_linux_amd64 -o kubebuilder
chmod +x kubebuilder
sudo mv kubebuilder /usr/local/bin/
```

Then run the tests with the integration tag:

```bash
go test -tags=integration ./...
```

## Common Issues

### "unsupported version: 2" Error
This error occurs when incompatible imports are used with Go 1.25.0. The solution is:

1. Use the compatibility layer for all client operations
2. Use helper functions to create reconcilers
3. Avoid direct imports of problematic packages:
   - `k8s.io/apimachinery/pkg/types`
   - `github.com/Azure/azure-sdk-for-go/sdk/azcore/to`

### Field Not Found Errors
Use getter methods instead of direct field access:

```go
// Incorrect
obj.Name

// Correct
obj.GetName()
```
