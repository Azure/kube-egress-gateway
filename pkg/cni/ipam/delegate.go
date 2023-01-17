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
package ipam

import (
	"errors"
	"fmt"

	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ipam"
)

type IPWrapper struct {
	pluginType string
	netConf    []byte
}

type IPProvider interface {
	WithIP(configFunc func(ipamResult *current.Result) error) (err error)
	DeleteIP() error
}

func New(pluginType string, netConf []byte) IPProvider {
	return &IPWrapper{
		pluginType: pluginType,
		netConf:    netConf,
	}
}

func (wrapper *IPWrapper) WithIP(configFunc func(ipamResult *current.Result) error) (err error) {
	r, err := ipam.ExecAdd(wrapper.pluginType, wrapper.netConf)
	if err != nil {
		return err
	}

	// release IP in case of failure
	defer func() {
		if err != nil {
			recoverErr := wrapper.DeleteIP()
			if recoverErr != nil {
				err = fmt.Errorf("error occured %w and failed to delete ip: %s", err, recoverErr.Error())
			}
		}
	}()
	ipamResult, err := current.NewResultFromResult(r)
	if err != nil {
		return err
	}

	if configFunc == nil {
		return errors.New("configure function is nil")
	}

	return configFunc(ipamResult)
}

func (wrapper *IPWrapper) DeleteIP() error {
	return ipam.ExecDel(wrapper.pluginType, wrapper.netConf)
}
