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

package main

import (
	"errors"
	"github.com/Azure/kube-egress-gateway/pkg/cni/conf"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/logger"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
	"net"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString(consts.KubeEgressIPAMCNIName))
}

func cmdAdd(args *skel.CmdArgs) error {
	log := logger.GetLogger()
	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		return err
	}

	// get k8s metadata
	k8sInfo, err := conf.LoadK8sInfo(args.Args)
	if err != nil {
		return err
	}
	log.V(5).Info("ADD - IPAM configuration successfully read: %+v", *k8sInfo)

	// allocate ip
	if config == nil || config.IPAM.Type == "" {
		return errors.New("ipam should not be empty")
	}

	podNetNS, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	defer podNetNS.Close()

	var address net.IPNet
	ipv6AddrFound := false
	var extraRoutes []*types.Route
	err = podNetNS.Do(func(netNS ns.NetNS) error {
		eth0Link, err := netlink.LinkByName("eth0")
		if err != nil {
			return err
		}
		addrList, err := netlink.AddrList(eth0Link, netlink.FAMILY_V6)
		if err != nil {
			return err
		}
		for _, item := range addrList {
			if item.Scope == unix.RT_SCOPE_LINK {
				address = *item.IPNet
				ipv6AddrFound = true
			}
		}
		routes, err := netlink.RouteList(eth0Link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}

		for _, route := range routes {
			if route.Dst == nil {
				for _, item := range config.ExcludedCIDRs {
					_, cidr, err := net.ParseCIDR(item)
					if err != nil {
						return err
					}
					extraRoutes = append(extraRoutes, &types.Route{
						Dst: *cidr,
						GW:  route.Gw,
					})
				}
				break
			}
		}

		return nil
	})
	if err != nil {
		return err
	}
	if !ipv6AddrFound {
		return errors.New("there is no enough ipv6 addr allocated for this pod")
	}
	result := &type100.Result{
		CNIVersion: type100.ImplementedSpecVersion,
		IPs: []*type100.IPConfig{
			{
				Address: address,
				Gateway: net.ParseIP(consts.GatewayIP),
			},
		},
		Routes: extraRoutes,
	}
	// outputCmdArgs(args)
	return types.PrintResult(result, config.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		return err
	}

	return types.PrintResult(&type100.Result{}, config.CNIVersion)
}

func cmdCheck(args *skel.CmdArgs) error {
	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		return err
	}

	return types.PrintResult(&type100.Result{}, config.CNIVersion)
}
