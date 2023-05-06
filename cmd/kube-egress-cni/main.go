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
	"os"

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

	// get prevResult
	result, err := type100.NewResultFromResult(config.PrevResult)
	if err != nil {
		return fmt.Errorf("failed to convert result to current version: %w", err)
	}

	// exchange public key with daemon
	conn, err := grpc.DialContext(context.Background(),
		config.SocketPath,
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

	// check if pod does not have gateway annotation, then skip the whole process
	resp, err := client.PodRetrieve(context.Background(), &v1.PodRetrieveRequest{
		PodConfig: &v1.PodInfo{
			PodName:      string(k8sInfo.K8S_POD_NAME),
			PodNamespace: string(k8sInfo.K8S_POD_NAMESPACE),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to get pod (%s/%s) annotations: %w", string(k8sInfo.K8S_POD_NAME), string(k8sInfo.K8S_POD_NAMESPACE), err)
	}
	annotations := resp.GetAnnotations()
	gwName, ok := annotations[consts.CNIGatewayAnnotationKey]
	if !ok {
		// pod does not use egress gateway, nothing else to do
		return types.PrintResult(result, config.CNIVersion)
	}

	// allocate ip
	if config == nil || config.IPAM.Type == "" {
		return errors.New("ipam should not be empty")
	}

	err = wireguard.WithWireGuardNic(args.ContainerID, args.Netns, consts.WireguardLinkName, ipam.New(config.IPAM.Type, args.StdinData), config.ExcludedCIDRs, result, func(podNs ns.NetNS, allowedIPNet string) error {
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
			wgDevice, err = wgclient.Device(consts.WireguardLinkName)
			if err != nil {
				return fmt.Errorf("failed to find wg device (%s): %w", consts.WireguardLinkName, err)
			}
			return nil
		})
		if err != nil {
			return err
		}

		resp, err := client.NicAdd(context.Background(), &v1.NicAddRequest{
			PodConfig: &v1.PodInfo{
				PodName:      string(k8sInfo.K8S_POD_NAME),
				PodNamespace: string(k8sInfo.K8S_POD_NAMESPACE),
			},
			PublicKey:   privateKey.PublicKey().String(),
			ListenPort:  int32(wgDevice.ListenPort),
			AllowedIp:   allowedIPNet,
			GatewayName: gwName,
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
			err = wgclient.ConfigureDevice(consts.WireguardLinkName, wgtypes.Config{
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

			exceptionsCidrs := append(resp.GetExceptionCidrs(), config.ExcludedCIDRs...)
			if os.Getenv("IS_UNIT_TEST_ENV") != "true" {
				if err := routes.SetPodRoutes(consts.WireguardLinkName, exceptionsCidrs, "/proc/sys", result); err != nil {
					return fmt.Errorf("failed to setup pod routes: %w", err)
				}
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
		config.SocketPath,
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

		ifHandle, err := netlink.LinkByName(consts.WireguardLinkName)
		if err != nil {
			//ignore error because cni delete may be invoked more than once.
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
