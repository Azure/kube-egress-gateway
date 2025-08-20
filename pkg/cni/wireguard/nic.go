// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package wireguard

import (
	"errors"
	"fmt"
	"os"

	current "github.com/containernetworking/cni/pkg/types/100"
	cniipam "github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"go.uber.org/multierr"

	"github.com/Azure/kube-egress-gateway/pkg/cni/ipam"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
)

type runner struct {
	netlink netlinkwrapper.Interface
	netns   netnswrapper.Interface
}

var nicRunner runner

func init() {
	nicRunner = runner{
		netlink: netlinkwrapper.NewNetLink(),
		netns:   netnswrapper.NewNetNS(),
	}
}

func WithWireGuardNic(containerID string, podNSPath string, ifName string, ipWrapper ipam.IPProvider, exludedRoute []string, result *current.Result, configFunc func(podNs ns.NetNS, allowedIPNet string) error) (err error) {
	podNetNS, err := nicRunner.netns.GetNSByPath(podNSPath)
	if err != nil {
		return err
	}
	defer func() { _ = podNetNS.Close() }()

	var wgLink netlink.Link

	// get existing interface in target ns
	err = podNetNS.Do(func(nn ns.NetNS) error {
		wgLink, err = nicRunner.netlink.LinkByName(ifName)
		if err != nil {
			if _, ok := err.(netlink.LinkNotFoundError); !ok {
				return fmt.Errorf("failed to retrieve new WireGuard link: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// if not found create one and move to pod ns
	if wgLink == nil {
		wgNameInMain := "wg" + containerID[0:8] // avoid name conflict in main ns
		linkAttributes := netlink.NewLinkAttrs()
		linkAttributes.Name = wgNameInMain
		wireguardInterface := &netlink.Wireguard{
			LinkAttrs: linkAttributes,
		}

		defer func() {
			if err != nil {
				if wgLink, recoverErr := nicRunner.netlink.LinkByName(wgNameInMain); recoverErr != nil {
					if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
						err = multierr.Append(err, recoverErr)
					}
				} else if recoverErr := nicRunner.netlink.LinkDel(wgLink); recoverErr != nil {
					err = multierr.Append(err, recoverErr)
				}

				recoverErr := podNetNS.Do(func(nn ns.NetNS) error {
					var err error
					if wgLink, recoverErr := nicRunner.netlink.LinkByName(wgNameInMain); recoverErr != nil {
						if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
							err = multierr.Append(err, recoverErr)
						}
					} else if recoverErr := nicRunner.netlink.LinkDel(wgLink); recoverErr != nil {
						err = multierr.Append(err, recoverErr)
					}
					if wgLink, recoverErr := nicRunner.netlink.LinkByName(ifName); recoverErr != nil {
						if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
							err = multierr.Append(err, recoverErr)
						}
					} else if recoverErr := nicRunner.netlink.LinkDel(wgLink); recoverErr != nil {
						err = multierr.Append(err, recoverErr)
					}
					return err
				})

				if recoverErr != nil {
					err = multierr.Append(err, recoverErr)
				}
			}
		}()

		err = nicRunner.netlink.LinkAdd(wireguardInterface)
		if err != nil {
			return fmt.Errorf("failed to add WireGuard link: %w", err)
		}
		wgLink, err = nicRunner.netlink.LinkByName(wgNameInMain)
		if err != nil {
			return err
		}
		err = nicRunner.netlink.LinkSetNsFd(wgLink, int(podNetNS.Fd()))
		if err != nil {
			return fmt.Errorf("failed to move wireguard link to pod namespace: %w", err)
		}
		err = podNetNS.Do(func(nn ns.NetNS) error {
			wgLink, err = nicRunner.netlink.LinkByName(wgNameInMain)
			if err != nil {
				return fmt.Errorf("failed to find %q: %v", wgNameInMain, err)
			}
			// Devices can be renamed only when down
			if err = nicRunner.netlink.LinkSetDown(wgLink); err != nil {
				return fmt.Errorf("failed to set %q down: %v", wgLink.Attrs().Name, err)
			}
			// Save host device name into the container device's alias property
			if err := nicRunner.netlink.LinkSetAlias(wgLink, wgLink.Attrs().Name); err != nil {
				return fmt.Errorf("failed to set alias to %q: %v", wgLink.Attrs().Name, err)
			}
			// Rename container device to respect args.IfName
			if err := nicRunner.netlink.LinkSetName(wgLink, ifName); err != nil {
				return fmt.Errorf("failed to rename device %q to %q: %v", wgLink.Attrs().Name, ifName, err)
			}
			// Bring container device up
			if err = nicRunner.netlink.LinkSetUp(wgLink); err != nil {
				return fmt.Errorf("failed to set %q up: %v", ifName, err)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	return ipWrapper.WithIP(func(ipamResult *current.Result) error {
		if ipamResult == nil || len(ipamResult.IPs) <= 0 {
			return errors.New("ipam result is empty")
		}

		allowedIPNet := ""
		err = podNetNS.Do(func(nn ns.NetNS) error {
			// Retrieve link again to get up-to-date name and attributes
			wgLink, err = nicRunner.netlink.LinkByName(ifName)
			if err != nil {
				return fmt.Errorf("failed to find %q: %v", ifName, err)
			}

			ipamResult.Interfaces = []*current.Interface{
				{
					Mac:     wgLink.Attrs().HardwareAddr.String(),
					Name:    wgLink.Attrs().Name,
					Sandbox: podNSPath,
				},
			}
			result.Interfaces = append(result.Interfaces, ipamResult.Interfaces[0])
			for _, item := range ipamResult.IPs {
				if item.Address.IP.To4() == nil {
					// add ipv6 ip to result
					item.Interface = current.Int(0)
					result.IPs = append(result.IPs, &current.IPConfig{
						Interface: current.Int(len(result.Interfaces) - 1),
						Address:   item.Address,
					})
				} else {
					// pod ipv4 ip should be added in wireguard configuration as allowed ip
					allowedIPNet = fmt.Sprintf("%s/32", item.Address.IP.String())
				}
			}
			if os.Getenv("IS_UNIT_TEST_ENV") != "true" {
				return cniipam.ConfigureIface(ifName, ipamResult)
			} else {
				return nil
			}
		})
		if err != nil {
			return err
		}

		if allowedIPNet == "" {
			return fmt.Errorf("failed to find pod ipv4 ip")
		}

		if configFunc != nil {
			return configFunc(podNetNS, allowedIPNet)
		}
		return nil
	})
}
