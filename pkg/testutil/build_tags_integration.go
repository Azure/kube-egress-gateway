// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

//go:build integration

// Package testutil provides utilities for testing
package testutil

// SkipIntegrationTests is false when the integration build tag is set
// This allows integration tests to run
var SkipIntegrationTests = false
