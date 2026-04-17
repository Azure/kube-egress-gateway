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
// The daemon runs with hostNetwork=true, so iptables commands see the host's
// network namespace.
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

// detectHostIPTablesMode determines which iptables backend the host is using.
// It tries multiple strategies in order:
//  1. Read /proc/1/root symlinks (requires SYS_PTRACE or permissive kernel)
//  2. Check for well-known kube-proxy rules in iptables-legacy-save output
//     (works because the daemon runs with hostNetwork=true)
//  3. Fall back to comparing rule counts across both backends
func detectHostIPTablesMode() (string, error) {
	// Strategy 1: Follow the host's iptables symlink chain via /proc/1/root.
	hostIPTables := filepath.Join(hostProcRoot, hostIPTablesPath)
	target, err := resolveSymlinkChain(hostIPTables, hostProcRoot)
	if err == nil {
		return classifyBinary(target), nil
	}

	// Strategy 2: Check whether kube-proxy's KUBE- chains exist in iptables-legacy.
	// The daemon runs with hostNetwork=true, so `iptables-legacy-save` sees the
	// host's network namespace. kube-proxy always creates KUBE- chains in whichever
	// backend the host uses. If KUBE- rules are present in legacy, the host uses
	// legacy; if absent, the host uses nft.
	mode, err := detectModeByKubeProxyChains()
	if err == nil {
		return mode, nil
	}

	// Strategy 3: Compare rule counts between legacy and nft save commands.
	return detectModeByRuleCounting()
}

// detectModeByKubeProxyChains checks whether kube-proxy's well-known chains
// (KUBE-SERVICES, KUBE-POSTROUTING) exist in the iptables-legacy nat table.
// This works because the daemon runs with hostNetwork=true.
func detectModeByKubeProxyChains() (string, error) {
	path, err := exec.LookPath(iptablesSaveLegacyBin)
	if err != nil {
		return "", fmt.Errorf("%s not found: %w", iptablesSaveLegacyBin, err)
	}
	out, err := exec.Command(path, "-t", "nat").Output() // #nosec G204 -- constant binary
	if err != nil {
		return "", fmt.Errorf("failed to run %s: %w", iptablesSaveLegacyBin, err)
	}

	output := string(out)
	// kube-proxy always creates these chains in the nat table.
	if strings.Contains(output, "KUBE-SERVICES") || strings.Contains(output, "KUBE-POSTROUTING") {
		return legacyBackend, nil
	}

	// KUBE- chains not found in legacy → host is using nft
	return nftBackend, nil
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
