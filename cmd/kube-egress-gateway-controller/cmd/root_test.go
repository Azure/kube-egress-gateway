// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package cmd

import "testing"

func TestBYOPrefixAuthorizationDefaultsToWarningOnly(t *testing.T) {
	flag := rootCmd.Flags().Lookup("enforce-byo-prefix-authorization")
	if flag == nil {
		t.Fatal("BYO prefix authorization flag is missing")
	}
	if flag.DefValue != "false" || enforceBYOPrefixAuthorization {
		t.Fatal("BYO prefix authorization must default to warning-only mode")
	}
}
