package netnswrapper

import "github.com/vishvananda/netns"

type Interface interface {
	// GetFromName gets a handle to a named network namespace
	GetFromName(name string) (netns.NsHandle, error)
	// DeleteNamed deletes a named network namespace
	DeleteNamed(name string) error
	// Get gets a handle to the current threads network namespace
	Get() (netns.NsHandle, error)
	// Set sets the current network namespace from ns handle
	Set(ns netns.NsHandle) error
	// NewNamed creates a new named network namespace and returns a handle to it
	NewNamed(name string) (netns.NsHandle, error)
}

type ns struct{}

func NewNetNS() Interface {
	return &ns{}
}

func (*ns) GetFromName(name string) (netns.NsHandle, error) {
	return netns.GetFromName(name)
}

func (*ns) DeleteNamed(name string) error {
	return netns.DeleteNamed(name)
}

func (*ns) Get() (netns.NsHandle, error) {
	return netns.Get()
}

func (*ns) Set(ns netns.NsHandle) error {
	return netns.Set(ns)
}

func (*ns) NewNamed(name string) (netns.NsHandle, error) {
	return netns.NewNamed(name)
}
