// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package iptableswrapper

import (
	"bytes"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/iptables"
	iptest "k8s.io/kubernetes/pkg/util/iptables/testing"
)

// FakeIPTables is no-op implementation of iptables Interface.
// We create a wrapper for iptest.FakeIPTables because the original package does not contain enough builtinTargets we need.
type FakeIPTables struct {
	fake           *iptest.FakeIPTables
	builtinTargets sets.Set[string]
}

// NewFake returns a no-op iptables.Interface
func NewFake() *FakeIPTables {
	return &FakeIPTables{
		fake:           iptest.NewFake(),
		builtinTargets: sets.New[string]("ACCEPT", "DROP", "RETURN", "REJECT", "DNAT", "SNAT", "MASQUERADE", "MARK", "CONNMARK"),
	}
}

// NewIPv6Fake returns a no-op iptables.Interface with IsIPv6() == true
func NewIPv6Fake() *FakeIPTables {
	return &FakeIPTables{
		fake:           iptest.NewIPv6Fake(),
		builtinTargets: sets.New[string]("ACCEPT", "DROP", "RETURN", "REJECT", "DNAT", "SNAT", "MASQUERADE", "MARK", "CONNMARK"),
	}
}

func (f *FakeIPTables) AddBuiltinTargets(targets ...string) {
	for _, target := range targets {
		f.builtinTargets = f.builtinTargets.Insert(target)
	}
}

// SetHasRandomFully sets f's return value for HasRandomFully()
func (f *FakeIPTables) SetHasRandomFully(can bool) *FakeIPTables {
	f.fake.SetHasRandomFully(can)
	return f
}

// EnsureChain is part of iptables.Interface
func (f *FakeIPTables) EnsureChain(table iptables.Table, chain iptables.Chain) (bool, error) {
	return f.fake.EnsureChain(table, chain)
}

// FlushChain is part of iptables.Interface
func (f *FakeIPTables) FlushChain(table iptables.Table, chain iptables.Chain) error {
	return f.fake.FlushChain(table, chain)
}

// DeleteChain is part of iptables.Interface
func (f *FakeIPTables) DeleteChain(table iptables.Table, chain iptables.Chain) error {
	return f.fake.DeleteChain(table, chain)
}

// ChainExists is part of iptables.Interface
func (f *FakeIPTables) ChainExists(table iptables.Table, chain iptables.Chain) (bool, error) {
	return f.fake.ChainExists(table, chain)
}

// EnsureRule is part of iptables.Interface
func (f *FakeIPTables) EnsureRule(position iptables.RulePosition, table iptables.Table, chain iptables.Chain, args ...string) (bool, error) {
	return f.fake.EnsureRule(position, table, chain, args...)
}

// DeleteRule is part of iptables.Interface
func (f *FakeIPTables) DeleteRule(table iptables.Table, chain iptables.Chain, args ...string) error {
	return f.fake.DeleteRule(table, chain, args...)
}

// IsIPv6 is part of iptables.Interface
func (f *FakeIPTables) IsIPv6() bool {
	return f.fake.IsIPv6()
}

// Protocol is part of iptables.Interface
func (f *FakeIPTables) Protocol() iptables.Protocol {
	return f.fake.Protocol()
}

// SaveInto is part of iptables.Interface
func (f *FakeIPTables) SaveInto(table iptables.Table, buffer *bytes.Buffer) error {
	return f.fake.SaveInto(table, buffer)
}

// copied from k8s.io/kubernetes/pkg/util/iptables/testing/fake.go with builtinTargets updated
func (f *FakeIPTables) restoreTable(newDump *iptest.IPTablesDump, newTable *iptest.Table, flush iptables.FlushFlag, counters iptables.RestoreCountersFlag) error {
	oldTable, err := f.fake.Dump.GetTable(newTable.Name)
	if err != nil {
		return err
	}

	backupChains := make([]iptest.Chain, len(oldTable.Chains))
	copy(backupChains, oldTable.Chains)

	// Update internal state
	if flush == iptables.FlushTables {
		oldTable.Chains = make([]iptest.Chain, 0, len(newTable.Chains))
	}
	for _, newChain := range newTable.Chains {
		oldChain, _ := f.fake.Dump.GetChain(newTable.Name, newChain.Name)
		switch {
		case oldChain == nil && newChain.Deleted:
			// no-op
		case oldChain == nil && !newChain.Deleted:
			oldTable.Chains = append(oldTable.Chains, newChain)
		case oldChain != nil && newChain.Deleted:
			_ = f.DeleteChain(newTable.Name, newChain.Name)
		case oldChain != nil && !newChain.Deleted:
			// replace old data with new
			oldChain.Rules = newChain.Rules
			if counters == iptables.RestoreCounters {
				oldChain.Packets = newChain.Packets
				oldChain.Bytes = newChain.Bytes
			}
		}
	}

	// Now check that all old/new jumps are valid
	for _, chain := range oldTable.Chains {
		for _, rule := range chain.Rules {
			if rule.Jump == nil {
				continue
			}
			if f.builtinTargets.Has(rule.Jump.Value) {
				continue
			}

			jumpedChain, _ := f.fake.Dump.GetChain(oldTable.Name, iptables.Chain(rule.Jump.Value))
			if jumpedChain == nil {
				newChain, _ := newDump.GetChain(oldTable.Name, iptables.Chain(rule.Jump.Value))
				if newChain != nil {
					// rule is an old rule that jumped to a chain which
					// was deleted by newDump.
					oldTable.Chains = backupChains
					return fmt.Errorf("deleted chain %q is referenced by existing rules", newChain.Name)
				} else {
					// rule is a new rule that jumped to a chain that was
					// neither created nor pre-existing
					oldTable.Chains = backupChains
					return fmt.Errorf("rule %q jumps to a non-existent chain", rule.Raw)
				}
			}
		}
	}

	return nil
}

// Restore is part of iptables.Interface
// copied from k8s.io/kubernetes/pkg/util/iptables/testing/fake.go
func (f *FakeIPTables) Restore(table iptables.Table, data []byte, flush iptables.FlushFlag, counters iptables.RestoreCountersFlag) error {
	dump, err := iptest.ParseIPTablesDump(string(data))
	if err != nil {
		return err
	}

	newTable, err := dump.GetTable(table)
	if err != nil {
		return err
	}

	return f.restoreTable(dump, newTable, flush, counters)
}

// RestoreAll is part of iptables.Interface
// copied from k8s.io/kubernetes/pkg/util/iptables/testing/fake.go
func (f *FakeIPTables) RestoreAll(data []byte, flush iptables.FlushFlag, counters iptables.RestoreCountersFlag) error {
	dump, err := iptest.ParseIPTablesDump(string(data))
	if err != nil {
		return err
	}

	for i := range dump.Tables {
		err = f.restoreTable(dump, &dump.Tables[i], flush, counters)
		if err != nil {
			return err
		}
	}
	return nil
}

// Monitor is part of iptables.Interface
func (f *FakeIPTables) Monitor(canary iptables.Chain, tables []iptables.Table, reloadFunc func(), interval time.Duration, stopCh <-chan struct{}) {
}

// HasRandomFully is part of iptables.Interface
func (f *FakeIPTables) HasRandomFully() bool {
	return f.fake.HasRandomFully()
}

func (f *FakeIPTables) Present() bool {
	return true
}

var _ = iptables.Interface(&FakeIPTables{})
