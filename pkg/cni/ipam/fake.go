// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package ipam

import current "github.com/containernetworking/cni/pkg/types/100"

type fakeIPProvider struct {
	result current.Result
}

func NewFakeIPProvider(ipamResult *current.Result) IPProvider {
	return &fakeIPProvider{result: *ipamResult}
}

func (fake *fakeIPProvider) WithIP(configFunc func(ipamResult *current.Result) error) (err error) {
	return configFunc(&fake.result)
}

func (*fakeIPProvider) DeleteIP() error {
	return nil
}
