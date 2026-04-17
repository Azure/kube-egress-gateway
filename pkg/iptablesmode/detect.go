// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package iptablesmode

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// iptables binary names
	iptablesLegacyBin        = "iptables-legacy"
	iptablesNftBin           = "iptables-nft"
	iptablesSaveLegacyBin    = "iptables-legacy-save"
	iptablesSaveNftBin       = "iptables-nft-save"
	iptablesRestoreLegacyBin = "iptables-legacy-restore"
	iptablesRestoreNftBin    = "iptables-nft-restore"

	// The path to the host's init process root filesystem, accessible when running with hostPID.
	hostProcRoot = "/proc/1/root"

	// Common paths where iptables binaries are found.
	hostIPTablesPath = "/usr/sbin/iptables"

	// Symlinks we need to update in the container.
	containerIPTables        = "/usr/sbin/iptables"
	containerIPTablesSave    = "/usr/sbin/iptables-save"
	containerIPTablesRestore = "/usr/sbin/iptables-restore"

	nftBackend    = "nft"
	legacyBackend = "legacy"
)

// DetectAndConfigureIPTablesMode detects the host's iptables backend (legacy vs nft)
// and configures the container's iptables symlinks to match.
// This must be called before any iptables interface is created.
// The daemon runs with hostPID=true, so /proc/1/root gives access to the host filesystem.
func DetectAndConfigureIPTablesMode() (string, error) {
	hostMode, err := detectHostIPTablesMode()
	if err != nil {
		return "", fmt.Errorf("failed to detect host iptables mode: %w", err)
	}

	containerMode, err := detectContainerIPTablesMode()
	if err != nil {
		return hostMode, fmt.Errorf("failed to detect container iptables mode: %w", err)
	}

	if hostMode == containerMode {
		return hostMode, nil
	}

	if err := switchIPTablesMode(hostMode); err != nil {
		return hostMode, fmt.Errorf("failed to switch iptables mode to %s: %w", hostMode, err)
	}

	return hostMode, nil
}

// detectHostIPTablesMode checks the host's iptables symlink to determine the backend.
// On some distros (e.g., Azure Linux 3), iptables uses the alternatives system:
//
//	/usr/sbin/iptables -> /etc/alternatives/iptables -> xtables-nft-multi
//
// We must follow the full chain to the final binary.
func detectHostIPTablesMode() (string, error) {
	hostIPTables := filepath.Join(hostProcRoot, hostIPTablesPath)

	// resolveSymlinkChain follows symlinks manually within the host's root filesystem.
	// filepath.EvalSymlinks cannot be used directly because intermediate symlinks
	// (e.g., /etc/alternatives/iptables) are absolute paths that only exist under
	// the host root (/proc/1/root), not in the container's own filesystem.
	target, err := resolveSymlinkChain(hostIPTables, hostProcRoot)
	if err != nil {
		// If we can't resolve the host symlink, fall back to rule counting
		return detectModeByRuleCounting()
	}

	return classifyBinary(target), nil
}

// resolveSymlinkChain follows a chain of symlinks rooted under hostRoot.
// Absolute symlink targets are re-rooted under hostRoot so the resolution
// stays within the host filesystem (accessed via /proc/1/root).
func resolveSymlinkChain(path, hostRoot string) (string, error) {
	const maxHops = 10
	for i := 0; i < maxHops; i++ {
		target, err := os.Readlink(path)
		if err != nil {
			// Not a symlink — this is the final file.
			return path, nil
		}
		if filepath.IsAbs(target) {
			// Absolute target: re-root under the host filesystem.
			path = filepath.Join(hostRoot, target)
		} else {
			// Relative target: resolve relative to the current link's directory.
			path = filepath.Join(filepath.Dir(path), target)
		}
	}
	return "", fmt.Errorf("too many levels of symlinks resolving %s", path)
}

// detectContainerIPTablesMode checks what the container's iptables points to.
func detectContainerIPTablesMode() (string, error) {
	path, err := exec.LookPath("iptables")
	if err != nil {
		return "", fmt.Errorf("iptables not found in PATH: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve iptables symlink: %w", err)
	}

	return classifyBinary(resolved), nil
}

// classifyBinary determines if a binary path indicates nft or legacy mode.
func classifyBinary(path string) string {
	base := filepath.Base(path)
	if strings.Contains(base, "nft") {
		return nftBackend
	}
	return legacyBackend
}

// detectModeByRuleCounting falls back to running both iptables-legacy-save and
// iptables-nft-save, counting which has more rules in the nat table.
func detectModeByRuleCounting() (string, error) {
	legacyCount := countIPTablesRules(iptablesSaveLegacyBin)
	nftCount := countIPTablesRules(iptablesSaveNftBin)

	if legacyCount > nftCount {
		return legacyBackend, nil
	}
	if nftCount > legacyCount {
		return nftBackend, nil
	}

	// If both are equal (e.g., both zero), default to legacy since AKS nodes
	// historically use iptables-legacy.
	return legacyBackend, nil
}

// countIPTablesRules runs the given save command and counts non-comment, non-empty lines.
func countIPTablesRules(saveCmd string) int {
	path, err := exec.LookPath(saveCmd)
	if err != nil {
		return 0
	}
	out, err := exec.Command(path).Output() // #nosec G204 -- saveCmd is a constant binary name
	if err != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "*") && !strings.HasPrefix(line, ":") && line != "COMMIT" {
			count++
		}
	}
	return count
}

// switchIPTablesMode updates the container's iptables/iptables-save/iptables-restore
// symlinks to point to the specified backend (legacy or nft).
func switchIPTablesMode(mode string) error {
	var targets map[string]string
	switch mode {
	case legacyBackend:
		targets = map[string]string{
			containerIPTables:        iptablesLegacyBin,
			containerIPTablesSave:    iptablesSaveLegacyBin,
			containerIPTablesRestore: iptablesRestoreLegacyBin,
		}
	case nftBackend:
		targets = map[string]string{
			containerIPTables:        iptablesNftBin,
			containerIPTablesSave:    iptablesSaveNftBin,
			containerIPTablesRestore: iptablesRestoreNftBin,
		}
	default:
		return fmt.Errorf("unknown iptables mode: %s", mode)
	}

	for symlink, target := range targets {
		targetPath, err := exec.LookPath(target)
		if err != nil {
			return fmt.Errorf("target binary %s not found: %w", target, err)
		}

		// Remove existing symlink/file and create new one.
		if err := os.Remove(symlink); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove existing %s: %w", symlink, err)
		}
		if err := os.Symlink(targetPath, symlink); err != nil {
			return fmt.Errorf("failed to create symlink %s -> %s: %w", symlink, targetPath, err)
		}
	}
	return nil
}
