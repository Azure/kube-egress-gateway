/*
MIT License

Copyright (c) Microsoft Corporation.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE
*/
package iptableswrapper

import "github.com/coreos/go-iptables/iptables"

type IpTables interface {
	// Exists checks if given rulespec in specified table/chain exists
	Exists(table, chain string, rulespec ...string) (bool, error)
	//Insert inserts given rulespec to specified table/chain at specified position
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
