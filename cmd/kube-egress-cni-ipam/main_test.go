// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"net"
	"strings"

	"github.com/containernetworking/cni/pkg/skel"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/containernetworking/plugins/pkg/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

const (
	ifName = "eth0"
)

func createCmdArgs(targetNS ns.NetNS) *skel.CmdArgs {
	conf := `{"cniVersion":"1.0.0","excludedCIDRs":["1.2.3.4/32","10.1.0.0/16"],"gatewayName":"test","ipam":{"type":"kube-egress-cni-ipam"},"name":"mynet","type":"kube-egress-cni"}`
	return &skel.CmdArgs{
		ContainerID: "test-container",
		Netns:       targetNS.Path(),
		IfName:      ifName,
		StdinData:   []byte(conf),
	}
}

var _ = Describe("Test kube-egress-cni-ipam operations", func() {
	var originalNS, targetNS ns.NetNS
	var args *skel.CmdArgs
	var ipv4Net, ipv6Net *net.IPNet

	BeforeEach(func() {
		var err error
		originalNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		targetNS, err = testutils.NewNS()
		Expect(err).NotTo(HaveOccurred())
		args = createCmdArgs(targetNS)
		ipv4Net, err = netlink.ParseIPNet("10.4.0.100/28")
		Expect(err).NotTo(HaveOccurred())
		ipv6Net, err = netlink.ParseIPNet("fe80::5/64")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(originalNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(originalNS)).To(Succeed())
		Expect(targetNS.Close()).To(Succeed())
		Expect(testutils.UnmountNS(targetNS)).To(Succeed())
	})

	It("should report error if eth0 is not found in cmdAdd", func() {
		err := originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).To(HaveOccurred())
			_, ok := err.(netlink.LinkNotFoundError)
			Expect(ok).To(BeTrue())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should report error when eth0 does not have ipv4 ip in cmdAdd", func() {
		err := targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			Expect(netlink.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: ifName}})).To(Succeed())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).To(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should report error when eth0 does not have ipv6 ip in cmdAdd", func() {
		err := targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			Expect(netlink.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: ifName}})).To(Succeed())
			eth0, err := netlink.LinkByName(ifName)
			Expect(err).NotTo(HaveOccurred())
			Expect(netlink.AddrAdd(eth0, &netlink.Addr{IPNet: ipv4Net})).To(Succeed())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			_, _, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).To(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should report appropriate ipv4 and ipv6 results in cmdAdd", func() {
		err := targetNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			Expect(netlink.LinkAdd(&netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: ifName}})).To(Succeed())
			eth0, err := netlink.LinkByName(ifName)
			Expect(err).NotTo(HaveOccurred())
			Expect(netlink.AddrAdd(eth0, &netlink.Addr{IPNet: ipv4Net})).To(Succeed())
			Expect(netlink.AddrAdd(eth0, &netlink.Addr{IPNet: ipv6Net})).To(Succeed())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		err = originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			r, raw, err := testutils.CmdAddWithArgs(args, func() error {
				return cmdAdd(args)
			})
			Expect(err).NotTo(HaveOccurred())
			resultType, err := r.GetAsVersion(type100.ImplementedSpecVersion)
			Expect(err).NotTo(HaveOccurred())
			result := resultType.(*type100.Result)
			Expect(len(result.IPs)).To(Equal(2))
			Expect(result.IPs[0].Address).To(Equal(*ipv6Net))
			Expect(result.IPs[1].Address).To(Equal(*ipv4Net))
			Expect(strings.Index(string(raw), "10.4.0.100/28")).Should(BeNumerically(">", 0))
			Expect(strings.Index(string(raw), "fe80::5/64")).Should(BeNumerically(">", 0))
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not report error in cmdDel", func() {
		err := originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			err := testutils.CmdDelWithArgs(args, func() error {
				return cmdDel(args)
			})
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not report error in cmdCheck", func() {
		err := originalNS.Do(func(ns.NetNS) error {
			defer GinkgoRecover()
			err := testutils.CmdCheckWithArgs(args, func() error {
				return cmdCheck(args)
			})
			Expect(err).NotTo(HaveOccurred())
			return nil
		})
		Expect(err).NotTo(HaveOccurred())
	})
})
