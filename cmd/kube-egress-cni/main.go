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
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/Azure/kube-egress-gateway/pkg/cni/routes"

	"github.com/Azure/kube-egress-gateway/pkg/cni/conf"
	"github.com/Azure/kube-egress-gateway/pkg/cni/ipam"
	"github.com/Azure/kube-egress-gateway/pkg/cni/wireguard"
	v1 "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	type100 "github.com/containernetworking/cni/pkg/types/100"
	"github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	bv "github.com/containernetworking/plugins/pkg/utils/buildversion"
	grpc_retry "github.com/grpc-ecosystem/go-grpc-middleware/retry"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, version.All, bv.BuildString(consts.KubeEgressCNIName))
}

func cmdAdd(args *skel.CmdArgs) error {

	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		return fmt.Errorf("failed to parse CNI config: %w", err)
	}

	// get k8s metadata
	k8sInfo, err := conf.LoadK8sInfo(args.Args)
	if err != nil {
		return fmt.Errorf("failed to load k8s metadata: %w", err)
	}

	// exchange public key with daemon
	conn, err := grpc.DialContext(context.Background(),
		consts.CNISocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", addr)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to contact cni manager daemon: %w", err)
	}
	defer conn.Close()
	client := v1.NewNicServiceClient(conn)

	// allocate ip
	if config == nil || config.IPAM.Type == "" {
		return errors.New("ipam should not be empty")
	}

	var result *type100.Result
	err = wireguard.WithWireGuardNic(args.ContainerID, args.Netns, args.IfName, ipam.New(config.IPAM.Type, args.StdinData), config.ExcludedCIDRs, func(podNs ns.NetNS, ipamResult *type100.Result) error {

		//generate private key
		privateKey, err := wgtypes.GeneratePrivateKey()
		if err != nil {
			return fmt.Errorf("failed to generate wg private key: %w", err)
		}
		var wgDevice *wgtypes.Device
		err = podNs.Do(func(nn ns.NetNS) error {
			wgclient, err := wgctrl.New()
			if err != nil {
				return fmt.Errorf("failed to create wg client: %w", err)
			}
			defer wgclient.Close()
			wgDevice, err = wgclient.Device(args.IfName)
			if err != nil {
				return fmt.Errorf("failed to find wg device (%s): %w", args.IfName, err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		var allowedIPs string
		for _, item := range ipamResult.IPs {
			// set pod's IPv4 address as allowed ip
			if item.Address.IP.To4() != nil {
				allowedIPs = fmt.Sprintf("%s/32", item.Address.IP.String())
			}
		}
		result = ipamResult
		resp, err := client.NicAdd(context.Background(), &v1.NicAddRequest{
			PodConfig: &v1.PodInfo{
				PodName:      string(k8sInfo.K8S_POD_NAME),
				PodNamespace: string(k8sInfo.K8S_POD_NAMESPACE),
			},
			PublicKey:   privateKey.PublicKey().String(),
			ListenPort:  int32(wgDevice.ListenPort),
			AllowedIp:   allowedIPs,
			GatewayName: config.GatewayName,
		})
		if err != nil {
			return fmt.Errorf("failed to send nicAdd request: %w", err)
		}

		gwPublicKey, err := wgtypes.ParseKey(resp.PublicKey)
		if err != nil {
			return fmt.Errorf("failed to parse gateway public key: %w", err)
		}

		return podNs.Do(func(nn ns.NetNS) error {
			wgclient, err := wgctrl.New()
			if err != nil {
				return fmt.Errorf("failed to create wg client: %w", err)
			}
			defer wgclient.Close()
			err = wgclient.ConfigureDevice(args.IfName, wgtypes.Config{
				PrivateKey: &privateKey,
				Peers: []wgtypes.PeerConfig{
					{
						PublicKey: gwPublicKey,
						Endpoint: &net.UDPAddr{
							IP:   net.ParseIP(resp.EndpointIp),
							Port: int(resp.ListenPort),
						},
						AllowedIPs: []net.IPNet{
							{
								IP:   net.IPv4zero,
								Mask: net.CIDRMask(0, 8*len(net.IPv4zero)),
							},
							{
								IP:   net.IPv6zero,
								Mask: net.CIDRMask(0, 8*len(net.IPv6zero)),
							},
						},
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to configure wg device: %w", err)

			}

			if err := routes.SetPodRoutes(args.IfName, config.ExcludedCIDRs); err != nil {
				return fmt.Errorf("failed to setup pod routes: %w", err)
			}
			return nil
		})
	})

	if err != nil {
		return err
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

	// get k8s metadata
	k8sInfo, err := conf.LoadK8sInfo(args.Args)
	if err != nil {
		return err
	}
	conn, err := grpc.DialContext(context.Background(),
		consts.CNISocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", addr)
		}))
	if err != nil {
		return err
	}
	defer conn.Close()
	client := v1.NewNicServiceClient(conn)

	podNs, err := ns.GetNS(args.Netns)
	if err != nil {
		return err
	}
	defer podNs.Close()
	err = podNs.Do(func(nn ns.NetNS) error {

		_, err = client.NicDel(context.Background(), &v1.NicDelRequest{
			PodConfig: &v1.PodInfo{
				PodName:      string(k8sInfo.K8S_POD_NAME),
				PodNamespace: string(k8sInfo.K8S_POD_NAMESPACE),
			},
		})
		if err != nil {
			return err
		}

		err = ipam.New(config.IPAM.Type, args.StdinData).DeleteIP()
		if err != nil {
			return err
		}

		ifHandle, err := netlink.LinkByName(args.IfName)
		if err != nil {
			//ignore error because cni delete may be invoked more than onece.
			return nil
		}
		return netlink.LinkDel(ifHandle)
	})
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
