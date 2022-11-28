package wgctrlwrapper

import (
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type Client interface {
	// Device retrieves a WireGuard device by its interface name
	Device(name string) (*wgtypes.Device, error)
	// ConfigureDevice configures a WireGuard device by its interface name
	ConfigureDevice(name string, cfg wgtypes.Config) error
	// Close releases resources used by a Client
	Close() error
}

type Interface interface {
	// New creates a new wireguard client
	New() (Client, error)
}

type wg struct{}

func NewWgCtrl() Interface {
	return &wg{}
}

func (*wg) New() (Client, error) {
	return wgctrl.New()
}
