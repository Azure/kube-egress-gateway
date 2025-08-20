// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package iptableswrapper

import "github.com/coreos/go-iptables/iptables"

type IpTables interface {
	// ApendUnique appends given rulespec if not exists
	AppendUnique(table, chain string, rulespec ...string) error
	// Exists checks if given rulespec in specified table/chain exists
	Exists(table, chain string, rulespec ...string) (bool, error)
	// Insert inserts given rulespec to specified table/chain at specified position
	Insert(table, chain string, pos int, rulespec ...string) error
	// Delete removes rulespec in specified table/chain
	Delete(table, chain string, rulespec ...string) error
	// List lists rules in specified table/chain
	List(table, chain string) ([]string, error)
}

type Interface interface {
	// New creates a new IpTables instance
	New() (IpTables, error)
}

type ipTable struct{}

func NewIPTables() Interface {
	return &ipTable{}
}

func (*ipTable) New() (IpTables, error) {
	return iptables.New()
}
