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
