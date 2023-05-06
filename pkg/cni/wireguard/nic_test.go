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
package wireguard

import (
	"errors"
	"net"
	"os"

	"github.com/Azure/kube-egress-gateway/pkg/cni/ipam"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper/mocknetlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper/mocknetnswrapper"
	current "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/vishvananda/netlink"
)

const (
	containerID  = "1234567890"
	podNSPath    = "/var/run/netns/cni-1234567890"
	nsName       = "cni-1234567890"
	ifName       = "wg0"
	ifNameInMain = "wg12345678"
)

func fakeConfigFunc(podNs ns.NetNS, allowedIPNet string) error {
	return nil
}

var _ = Describe("test WithWireGuardNic", func() {
	var ipamResult current.Result
	BeforeEach(func() {
		mctrl := gomock.NewController(GinkgoT())
		nicRunner = runner{
			netlink: mocknetlinkwrapper.NewMockInterface(mctrl),
			netns:   mocknetnswrapper.NewMockInterface(mctrl),
		}
		ipamResult = current.Result{
			IPs: []*current.IPConfig{
				{Address: net.IPNet{IP: net.ParseIP("10.0.0.4"), Mask: net.CIDRMask(24, 32)}},
				{Address: net.IPNet{IP: net.ParseIP("fe80::1234"), Mask: net.CIDRMask(128, 128)}},
			},
		}
		os.Setenv("IS_UNIT_TEST_ENV", "true")
	})

	AfterEach(func() {
		os.Setenv("IS_UNIT_TEST_ENV", "")
	})

	It("should return error when pod network namespace is not found", func() {
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mns.EXPECT().GetNSByPath(podNSPath).Return(nil, os.ErrNotExist)
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, nil, fakeConfigFunc)
		Expect(err).To(HaveOccurred())
	})

	It("should return error if getting wireguard interface returns error", func() {
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mlink := nicRunner.netlink.(*mocknetlinkwrapper.MockInterface)
		gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
		gomock.InOrder(
			mns.EXPECT().GetNSByPath(podNSPath).Return(gwns, nil),
			mlink.EXPECT().LinkByName(ifName).Return(nil, errors.New("error")),
		)
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, nil, fakeConfigFunc)
		Expect(err).To(HaveOccurred())
	})

	It("should create wglink and return result from ipam", func() {
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mlink := nicRunner.netlink.(*mocknetlinkwrapper.MockInterface)
		gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
		wgMain := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifNameInMain}}
		wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifName}}
		gomock.InOrder(
			mns.EXPECT().GetNSByPath(podNSPath).Return(gwns, nil),
			mlink.EXPECT().LinkByName(ifName).Return(nil, netlink.LinkNotFoundError{}),
			mlink.EXPECT().LinkAdd(&netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{
				NetNsID: -1,
				TxQLen:  -1,
				Name:    ifNameInMain,
			}}).Return(nil),
			mlink.EXPECT().LinkByName(ifNameInMain).Return(wgMain, nil),
			mlink.EXPECT().LinkSetNsFd(wgMain, int(gwns.Fd())),
			mlink.EXPECT().LinkByName(ifNameInMain).Return(wgMain, nil),
			mlink.EXPECT().LinkSetDown(wgMain).Return(nil),
			mlink.EXPECT().LinkSetAlias(wgMain, ifNameInMain).Return(nil),
			mlink.EXPECT().LinkSetName(wgMain, ifName).Return(nil),
			mlink.EXPECT().LinkSetUp(wgMain).Return(nil),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
		)
		result := &current.Result{}
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, result, fakeConfigFunc)
		Expect(result).To(Equal(&current.Result{
			Interfaces: []*current.Interface{
				{
					Mac:     "",
					Name:    ifName,
					Sandbox: podNSPath,
				},
			},
			IPs: []*current.IPConfig{
				{
					Interface: current.Int(0),
					Address:   net.IPNet{IP: net.ParseIP("fe80::1234"), Mask: net.CIDRMask(128, 128)},
				},
			},
		}))
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not recreate wglink when it already exists", func() {
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mlink := nicRunner.netlink.(*mocknetlinkwrapper.MockInterface)
		gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
		wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifName}}
		gomock.InOrder(
			mns.EXPECT().GetNSByPath(podNSPath).Return(gwns, nil),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
		)
		result := &current.Result{}
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, result, fakeConfigFunc)
		Expect(result).To(Equal(&current.Result{
			Interfaces: []*current.Interface{
				{
					Mac:     "",
					Name:    ifName,
					Sandbox: podNSPath,
				},
			},
			IPs: []*current.IPConfig{
				{
					Interface: current.Int(0),
					Address:   net.IPNet{IP: net.ParseIP("fe80::1234"), Mask: net.CIDRMask(128, 128)},
				},
			},
		}))
		Expect(err).NotTo(HaveOccurred())
	})

	It("should recover changes when encountering any error", func() {
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mlink := nicRunner.netlink.(*mocknetlinkwrapper.MockInterface)
		gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
		wgMain := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifNameInMain}}
		wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifName}}
		gomock.InOrder(
			mns.EXPECT().GetNSByPath(podNSPath).Return(gwns, nil),
			mlink.EXPECT().LinkByName(ifName).Return(nil, netlink.LinkNotFoundError{}),
			mlink.EXPECT().LinkAdd(&netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{
				NetNsID: -1,
				TxQLen:  -1,
				Name:    ifNameInMain,
			}}).Return(errors.New("failed")),
			// clean up garbage interface in host namespace
			mlink.EXPECT().LinkByName(ifNameInMain).Return(wgMain, nil),
			mlink.EXPECT().LinkDel(wgMain).Return(nil),
			// clean up garbage interface in pod network namespace
			mlink.EXPECT().LinkByName(ifNameInMain).Return(nil, netlink.LinkNotFoundError{}),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
			mlink.EXPECT().LinkDel(wg0).Return(nil),
		)
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, nil, fakeConfigFunc)
		Expect(err).To(HaveOccurred())
	})

	It("should return error if pod ipv4 ip is not found", func() {
		ipamResult.IPs = ipamResult.IPs[1:]
		mns := nicRunner.netns.(*mocknetnswrapper.MockInterface)
		mlink := nicRunner.netlink.(*mocknetlinkwrapper.MockInterface)
		gwns := &mocknetnswrapper.MockNetNS{Name: nsName}
		wg0 := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: ifName}}
		gomock.InOrder(
			mns.EXPECT().GetNSByPath(podNSPath).Return(gwns, nil),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
			mlink.EXPECT().LinkByName(ifName).Return(wg0, nil),
		)
		result := &current.Result{}
		err := WithWireGuardNic(containerID, podNSPath, ifName, ipam.NewFakeIPProvider(&ipamResult), []string{}, result, fakeConfigFunc)
		Expect(err).To(HaveOccurred())
	})
})
