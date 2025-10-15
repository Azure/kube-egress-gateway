// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

//go:build !integration

// Package testutil provides utilities for testing
package testutil

// SkipIntegrationTests is a marker variable used to skip integration tests
// This package is built only when the integration build tag is not set
var SkipIntegrationTests = true
