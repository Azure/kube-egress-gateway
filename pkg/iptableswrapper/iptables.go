package iptableswrapper

import "github.com/coreos/go-iptables/iptables"

type IpTables interface {
	// AppendUnique appends give rulespec to specified table/chain if it does not exist
	AppendUnique(table, chain string, rulespec ...string) error
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
