package wireguard

import (
	"errors"
	"fmt"

	"github.com/Azure/kube-egress-gateway/pkg/cni/ipam"
	current "github.com/containernetworking/cni/pkg/types/100"
	cniipam "github.com/containernetworking/plugins/pkg/ipam"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/vishvananda/netlink"
	"go.uber.org/multierr"
)

func WithWireGuardNic(containerID string, podNSPath string, ifName string, ipWrapper ipam.IPProvider, exludedRoute []string, configFunc func(podNs ns.NetNS, ipamResult *current.Result) error) (err error) {
	// Lock the thread so we don't hop goroutines and namespaces

	podNetNS, err := ns.GetNS(podNSPath)
	if err != nil {
		return err
	}
	defer podNetNS.Close()

	var wgLink netlink.Link

	// get existing interface in target ns
	err = podNetNS.Do(func(nn ns.NetNS) error {
		wgLink, err = netlink.LinkByName(ifName)
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

	//if not found create one and move to pod ns
	if wgLink == nil {
		wgNameInMain := "wg" + containerID[0:8] //avoid name conflict in main ns
		linkAttributes := netlink.NewLinkAttrs()
		linkAttributes.Name = wgNameInMain
		wireguardInterface := &netlink.Wireguard{
			LinkAttrs: linkAttributes,
		}

		err = netlink.LinkAdd(wireguardInterface)
		if err != nil {
			return fmt.Errorf("failed to add WireGuard link: %w", err)
		}
		wgLink, err = netlink.LinkByName(wgNameInMain)
		if err != nil {
			return err
		}
		err = netlink.LinkSetNsFd(wgLink, int(podNetNS.Fd()))
		if err != nil {
			return fmt.Errorf("failed to move wireguard link to pod namespace: %w", err)
		}
		err = podNetNS.Do(func(nn ns.NetNS) error {
			wgLink, err = netlink.LinkByName(wgNameInMain)
			if err != nil {
				return fmt.Errorf("failed to find %q: %v", wgNameInMain, err)
			}
			// Devices can be renamed only when down
			if err = netlink.LinkSetDown(wgLink); err != nil {
				return fmt.Errorf("failed to set %q down: %v", wgLink.Attrs().Name, err)
			}
			// Save host device name into the container device's alias property
			if err := netlink.LinkSetAlias(wgLink, wgLink.Attrs().Name); err != nil {
				return fmt.Errorf("failed to set alias to %q: %v", wgLink.Attrs().Name, err)
			}
			// Rename container device to respect args.IfName
			if err := netlink.LinkSetName(wgLink, ifName); err != nil {
				return fmt.Errorf("failed to rename device %q to %q: %v", wgLink.Attrs().Name, ifName, err)
			}
			// Bring container device up
			if err = netlink.LinkSetUp(wgLink); err != nil {
				return fmt.Errorf("failed to set %q up: %v", ifName, err)
			}
			// Retrieve link again to get up-to-date name and attributes
			wgLink, err = netlink.LinkByName(ifName)
			if err != nil {
				return fmt.Errorf("failed to find %q: %v", ifName, err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		defer func() {
			if err != nil {
				if wgLink, recoverErr := netlink.LinkByName(wgNameInMain); recoverErr != nil {
					if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
						err = multierr.Append(err, recoverErr)
					}
				} else if recoverErr := netlink.LinkDel(wgLink); recoverErr != nil {
					err = multierr.Append(err, recoverErr)
				}

				recoverErr := podNetNS.Do(func(nn ns.NetNS) error {
					var err error
					if wgLink, recoverErr := netlink.LinkByName(wgNameInMain); recoverErr != nil {
						if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
							err = multierr.Append(err, recoverErr)
						}
					} else if recoverErr := netlink.LinkDel(wgLink); recoverErr != nil {
						err = multierr.Append(err, recoverErr)
					}
					if wgLink, recoverErr := netlink.LinkByName(wgNameInMain); recoverErr != nil {
						if _, ok := recoverErr.(netlink.LinkNotFoundError); !ok {
							err = multierr.Append(err, recoverErr)
						}
					} else if recoverErr := netlink.LinkDel(wgLink); recoverErr != nil {
						err = multierr.Append(err, recoverErr)
					}
					return err
				})

				if recoverErr != nil {
					err = multierr.Append(err, recoverErr)
				}
			}
		}()
	}

	return ipWrapper.WithIP(func(ipamResult *current.Result) error {
		if ipamResult == nil || len(ipamResult.IPs) <= 0 {
			return errors.New("ipam result is empty")
		}

		ipamResult.Interfaces = []*current.Interface{
			{
				Mac:     wgLink.Attrs().HardwareAddr.String(),
				Name:    wgLink.Attrs().Name,
				Sandbox: podNSPath,
			},
		}
		for _, item := range ipamResult.IPs {
			item.Interface = current.Int(0)
		}

		err = podNetNS.Do(func(nn ns.NetNS) error {
			return cniipam.ConfigureIface(ifName, ipamResult)
		})
		if err != nil {
			return err
		}

		if configFunc != nil {
			return configFunc(podNetNS, ipamResult)
		}
		return nil
	})
}
