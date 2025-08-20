// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package mocknetnswrapper

import "github.com/containernetworking/plugins/pkg/ns"

// MockNetNS: implements ns.NetNS interface
type MockNetNS struct {
	Name string
}

func (ns *MockNetNS) Do(toRun func(ns.NetNS) error) error {
	return toRun(ns)
}

func (*MockNetNS) Set() error {
	return nil
}

func (*MockNetNS) Path() string {
	return ""
}

func (*MockNetNS) Fd() uintptr {
	return 0
}

func (*MockNetNS) Close() error {
	return nil
}
