// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package iptablesmode

import (
	"testing"
)

func TestClassifyBinary(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "nft multi binary",
			path:     "/usr/sbin/xtables-nft-multi",
			expected: nftBackend,
		},
		{
			name:     "legacy multi binary",
			path:     "/usr/sbin/xtables-legacy-multi",
			expected: legacyBackend,
		},
		{
			name:     "iptables-nft symlink",
			path:     "/usr/sbin/iptables-nft",
			expected: nftBackend,
		},
		{
			name:     "iptables-legacy symlink",
			path:     "/usr/sbin/iptables-legacy",
			expected: legacyBackend,
		},
		{
			name:     "plain iptables binary",
			path:     "/usr/sbin/iptables",
			expected: legacyBackend,
		},
		{
			name:     "nft in nested path",
			path:     "/some/path/xtables-nft-multi",
			expected: nftBackend,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyBinary(tt.path)
			if result != tt.expected {
				t.Errorf("classifyBinary(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestCountIPTablesRules(t *testing.T) {
	// Test with a non-existent binary - should return 0
	count := countIPTablesRules("nonexistent-binary-12345")
	if count != 0 {
		t.Errorf("countIPTablesRules with non-existent binary = %d, want 0", count)
	}
}
