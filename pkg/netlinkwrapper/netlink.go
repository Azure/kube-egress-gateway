package netlinkwrapper

import "github.com/vishvananda/netlink"

type Interface interface {
	// LinkByName finds a link by name
	LinkByName(name string) (netlink.Link, error)
	// LinkAdd adds a new link device
	LinkAdd(link netlink.Link) error
	// LinkDel deletes link device
	LinkDel(link netlink.Link) error
	// LinkSetUp enables the link device
	LinkSetUp(link netlink.Link) error
	// LinkSetNsFd puts the device into a new network namespace
	LinkSetNsFd(link netlink.Link, fd int) error
	// AddrList gets a list of IP addresses in the system
	AddrList(link netlink.Link, family int) ([]netlink.Addr, error)
	// AddrAdd adds an IP address to a link device
	AddrAdd(link netlink.Link, addr *netlink.Addr) error
	// AddrDel deletes an IP address from a link device
	AddrDel(link netlink.Link, addr *netlink.Addr) error
	// AddrReplace replaces (or, if not present, adds) an IP address on a link device
	AddrReplace(link netlink.Link, addr *netlink.Addr) error
	// RouteReplace adds a route to the system
	RouteReplace(route *netlink.Route) error
	// RouteDel deletes a route from the system
	RouteDel(route *netlink.Route) error
	// RouteList gets a list of routes in the system
	RouteList(link netlink.Link, family int) ([]netlink.Route, error)
}

type nl struct{}

func NewNetLink() Interface {
	return &nl{}
}

func (*nl) LinkByName(name string) (netlink.Link, error) {
	return netlink.LinkByName(name)
}

func (*nl) LinkAdd(link netlink.Link) error {
	return netlink.LinkAdd(link)
}

func (*nl) LinkDel(link netlink.Link) error {
	return netlink.LinkDel(link)
}

func (*nl) LinkSetUp(link netlink.Link) error {
	return netlink.LinkSetUp(link)
}

func (*nl) LinkSetNsFd(link netlink.Link, fd int) error {
	return netlink.LinkSetNsFd(link, fd)
}

func (*nl) AddrList(link netlink.Link, family int) ([]netlink.Addr, error) {
	return netlink.AddrList(link, family)
}

func (*nl) AddrAdd(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrAdd(link, addr)
}

func (*nl) AddrDel(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrDel(link, addr)
}

func (*nl) AddrReplace(link netlink.Link, addr *netlink.Addr) error {
	return netlink.AddrReplace(link, addr)
}

func (*nl) RouteReplace(route *netlink.Route) error {
	return netlink.RouteReplace(route)
}

func (*nl) RouteDel(route *netlink.Route) error {
	return netlink.RouteDel(route)
}

func (*nl) RouteList(link netlink.Link, family int) ([]netlink.Route, error) {
	return netlink.RouteList(link, family)
}
