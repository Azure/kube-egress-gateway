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
package routes

import (
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/iptableswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

type Interface interface {
	SetPodRoutes(ifName string, exceptionCidrs []string) error
}

type runner struct {
	netlink  netlinkwrapper.Interface
	iptables iptableswrapper.Interface
}

var routesRunner runner

func init() {
	routesRunner = runner{
		netlink:  netlinkwrapper.NewNetLink(),
		iptables: iptableswrapper.NewIPTables(),
	}
}

func SetPodRoutes(ifName string, exceptionCidrs []string) error {
	// 1. removes existing routes
	// 2. add original default route gateway to eth0
	// 3. routes exceptional cidrs (traffic avoiding gateway) to base interface (eth0)
	// 4. add default route to wireguard interface
	eth0Link, err := routesRunner.netlink.LinkByName("eth0")
	if err != nil {
		return fmt.Errorf("failed to retrieve eth0 interface: %w", err)
	}

	routes, err := routesRunner.netlink.RouteList(eth0Link, nl.FAMILY_ALL)
	if err != nil {
		return fmt.Errorf("failed to list all routes on eth0: %w", err)
	}

	var defaultRoute *netlink.Route
	for _, route := range routes {
		route := route
		if route.Dst == nil && route.Family == nl.FAMILY_V4 {
			defaultRoute = &route
		}
		if err := routesRunner.netlink.RouteDel(&route); err != nil {
			return fmt.Errorf("failed to delete route (%s): %w", route, err)
		}
	}

	if defaultRoute == nil {
		return errors.New("failed to find default route")
	}

	err = routesRunner.netlink.RouteReplace(&netlink.Route{
		Dst:       &net.IPNet{IP: defaultRoute.Gw, Mask: net.CIDRMask(32, 32)},
		LinkIndex: eth0Link.Attrs().Index,
		Scope:     netlink.SCOPE_LINK,
	})
	if err != nil {
		return fmt.Errorf("failed to add original gateway route: %w", err)
	}

	for _, exception := range exceptionCidrs {
		_, cidr, err := net.ParseCIDR(exception)
		if err != nil {
			return fmt.Errorf("failed to parse cidr (%s): %w", exception, err)
		}
		gatewayRoute := netlink.Route{
			Dst:       cidr,
			Gw:        defaultRoute.Gw,
			LinkIndex: eth0Link.Attrs().Index,
			Protocol:  unix.RTPROT_STATIC,
		}
		err = routesRunner.netlink.RouteReplace(&gatewayRoute)
		if err != nil {
			return fmt.Errorf("failed to add route (%s): %w", gatewayRoute, err)
		}
	}

	wgLink, err := routesRunner.netlink.LinkByName(ifName)
	if err != nil {
		return fmt.Errorf("failed to retrieve wireguard interface: %w", err)
	}

	_, default_route_cidr, _ := net.ParseCIDR("0.0.0.0/0")
	wg_default_route := netlink.Route{
		Dst: default_route_cidr,
		Gw:  nil,
		Via: &netlink.Via{
			Addr:       net.ParseIP("fe80::1"),
			AddrFamily: nl.FAMILY_V6,
		},
		LinkIndex: wgLink.Attrs().Index,
		Scope:     netlink.SCOPE_UNIVERSE,
		Family:    nl.FAMILY_V4,
	}

	err = routesRunner.netlink.RouteReplace(&wg_default_route)
	if err != nil {
		return fmt.Errorf("failed to add default wireguard route (%s): %w", wg_default_route, err)
	}

	err = addRoutingForIngress(eth0Link, *defaultRoute)
	if err != nil {
		return err
	}
	return nil
}

func addRoutingForIngress(eth0Link netlink.Link, defaultRoute netlink.Route) error {
	// add iptables rule to mark traffic from eth0
	ipt, err := routesRunner.iptables.New()
	if err != nil {
		return fmt.Errorf("failed to create iptable: %w", err)
	}
	err = ipt.AppendUnique(consts.MangleTable, consts.PreRoutingChain, "-i", "eth0", "-j", "MARK", "--set-mark", strconv.Itoa(consts.Eth0Mark))
	if err != nil {
		return fmt.Errorf("failed to append iptables set-mark rule: %w", err)
	}
	err = ipt.AppendUnique(consts.MangleTable, consts.PreRoutingChain, "-j", "CONNMARK", "--save-mark")
	if err != nil {
		return fmt.Errorf("failed to append iptables save-mark rule: %w", err)
	}
	err = ipt.AppendUnique(consts.MangleTable, consts.OutputChain, "-m", "connmark", "--mark", strconv.Itoa(consts.Eth0Mark), "-j", "CONNMARK", "--restore-mark")
	if err != nil {
		return fmt.Errorf("failed to append iptables restore-mark rule: %w", err)
	}

	// add ip rule: lookup separate routing table if packet is marked
	rule := netlink.NewRule()
	rule.Mark = consts.Eth0Mark
	rule.Table = consts.Eth0Mark
	err = routesRunner.netlink.RuleAdd(rule)
	if err != nil {
		return fmt.Errorf("failed to add routing rule: %w", err)
	}

	// add route
	defaultRoute.Table = consts.Eth0Mark
	err = routesRunner.netlink.RouteReplace(&defaultRoute)
	if err != nil {
		return fmt.Errorf("failed to add default route via eth0: %w", err)
	}
	return nil
}
