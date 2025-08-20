// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"errors"
	"net"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/Azure/kube-egress-gateway/pkg/cni/conf"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/logger"
)

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Check: cmdCheck, Del: cmdDel}, version.All, bv.BuildString(consts.KubeEgressIPAMCNIName))
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
	defer func() {
		if err := podNetNS.Close(); err != nil {
			// Log error but don't fail the operation
			log.Error(err, "failed to close pod network namespace")
		}
	}()

	var v4Address, v6Address net.IPNet
	var ipv4AddrFound, ipv6AddrFound bool
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
				v6Address = *item.IPNet
				ipv6AddrFound = true
			}
		}

		addrList, err = netlink.AddrList(eth0Link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		for _, item := range addrList {
			if item.Scope == unix.RT_SCOPE_UNIVERSE {
				v4Address = *item.IPNet
				ipv4AddrFound = true
			}
		}
		return nil
	})

	if err != nil {
		return err
	}
	if !ipv4AddrFound {
		return errors.New("there is no enough ipv4 addr allocated for this pod")
	}
	if !ipv6AddrFound {
		return errors.New("there is no enough ipv6 addr allocated for this pod")
	}

	result := &type100.Result{
		CNIVersion: type100.ImplementedSpecVersion,
		IPs: []*type100.IPConfig{
			{
				Address: v6Address,
				Gateway: net.ParseIP(consts.GatewayIP),
			},
			{
				Address: v4Address,
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
