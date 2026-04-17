// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package iptablesmode

import (
	"os"
	"path/filepath"
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

func TestResolveSymlinkChain(t *testing.T) {
	// Create a temp directory to simulate host filesystem
	tmpDir := t.TempDir()

	// Simulate Azure Linux 3 alternatives chain:
	// /usr/sbin/iptables -> /etc/alternatives/iptables -> /usr/sbin/xtables-nft-multi (real file)
	usrSbin := filepath.Join(tmpDir, "usr", "sbin")
	etcAlt := filepath.Join(tmpDir, "etc", "alternatives")
	if err := os.MkdirAll(usrSbin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(etcAlt, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create the real binary
	realBin := filepath.Join(usrSbin, "xtables-nft-multi")
	if err := os.WriteFile(realBin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	// /etc/alternatives/iptables -> /usr/sbin/xtables-nft-multi (absolute symlink)
	if err := os.Symlink("/usr/sbin/xtables-nft-multi", filepath.Join(etcAlt, "iptables")); err != nil {
		t.Fatal(err)
	}

	// /usr/sbin/iptables -> /etc/alternatives/iptables (absolute symlink)
	if err := os.Symlink("/etc/alternatives/iptables", filepath.Join(usrSbin, "iptables")); err != nil {
		t.Fatal(err)
	}

	// Resolve starting from the "host root"
	resolved, err := resolveSymlinkChain(filepath.Join(tmpDir, "usr", "sbin", "iptables"), tmpDir)
	if err != nil {
		t.Fatalf("resolveSymlinkChain failed: %v", err)
	}

	if classifyBinary(resolved) != nftBackend {
		t.Errorf("expected nft backend from resolved path %q", resolved)
	}

	// Test direct legacy symlink (Azure Linux 2 style):
	// /usr/sbin/iptables -> xtables-legacy-multi (relative symlink)
	tmpDir2 := t.TempDir()
	usrSbin2 := filepath.Join(tmpDir2, "usr", "sbin")
	if err := os.MkdirAll(usrSbin2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(usrSbin2, "xtables-legacy-multi"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("xtables-legacy-multi", filepath.Join(usrSbin2, "iptables")); err != nil {
		t.Fatal(err)
	}

	resolved2, err := resolveSymlinkChain(filepath.Join(usrSbin2, "iptables"), tmpDir2)
	if err != nil {
		t.Fatalf("resolveSymlinkChain failed: %v", err)
	}
	if classifyBinary(resolved2) != legacyBackend {
		t.Errorf("expected legacy backend from resolved path %q", resolved2)
	}
}
