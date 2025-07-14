// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

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
	"k8s.io/klog/v2"

	"github.com/Azure/kube-egress-gateway/pkg/cni/conf"
	"github.com/Azure/kube-egress-gateway/pkg/cni/ipam"
	"github.com/Azure/kube-egress-gateway/pkg/cni/routes"
	"github.com/Azure/kube-egress-gateway/pkg/cni/wireguard"
	v1 "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
)

func main() {
	skel.PluginMainFuncs(skel.CNIFuncs{Add: cmdAdd, Check: cmdCheck, Del: cmdDel}, version.All, bv.BuildString(consts.KubeEgressCNIName))
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
	conn, err := grpc.NewClient(config.SocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp", addr)
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to contact cni manager daemon: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			// Log error but don't fail the operation
			klog.ErrorS(err, "failed to close gRPC connection")
		}
	}()
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
			defer func() {
				if err := wgclient.Close(); err != nil {
					// Log error but don't fail the operation
					klog.ErrorS(err, "failed to close wireguard client")
				}
			}()
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
			defer func() {
				if err := wgclient.Close(); err != nil {
					// Log error but don't fail the operation
					klog.ErrorS(err, "failed to close wireguard client")
				}
			}()
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
			defaultToGateway := resp.GetDefaultRoute() == v1.DefaultRoute_DEFAULT_ROUTE_STATIC_EGRESS_GATEWAY
			if os.Getenv("IS_UNIT_TEST_ENV") != "true" {
				if err := routes.SetPodRoutes(consts.WireguardLinkName, exceptionsCidrs, defaultToGateway, "/proc/sys", result); err != nil {
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
	logger := klog.NewKlogr().WithName("kube-egress-cni").WithValues("containerID", args.ContainerID, "netns", args.Netns, "ifname", args.IfName)
	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		logger.Error(err, "failed to parse CNI config")
		return nil
	}

	// get k8s metadata
	k8sInfo, err := conf.LoadK8sInfo(args.Args)
	if err != nil {
		logger.Error(err, "failed to load k8s metadata")
		return nil
	}
	podNs, err := ns.GetNS(args.Netns)
	if err != nil {
		logger.Error(err, "failed to get pod namespace")
		return nil
	}
	defer func() {
		if err := podNs.Close(); err != nil {
			logger.Error(err, "failed to close pod namespace")
		}
	}()
	err = podNs.Do(func(nn ns.NetNS) error {
		ifHandle, err := netlink.LinkByName(consts.WireguardLinkName)
		if err != nil {
			//ignore error because cni delete may be invoked more than once.
			return nil
		}
		if err := netlink.LinkDel(ifHandle); err != nil {
			logger.Error(err, "failed to delete wireguard link")
		}
		return nil
	})
	if err != nil {
		logger.Error(err, "failed to delete wireguard link")
		return nil
	}

	err = ipam.New(config.IPAM.Type, args.StdinData).DeleteIP()
	if err != nil {
		logger.Error(err, "failed to delete ip")
		return nil
	}
	conn, err := grpc.NewClient(config.SocketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStreamInterceptor(grpc_retry.StreamClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_retry.UnaryClientInterceptor()),
		grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "tcp", addr)
		}))
	if err != nil {
		logger.Error(err, "failed to contact cni manager daemon")
		return nil
	}
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Error(err, "failed to close connection")
		}
	}()
	client := v1.NewNicServiceClient(conn)
	_, err = client.NicDel(context.Background(), &v1.NicDelRequest{
		PodConfig: &v1.PodInfo{
			PodName:      string(k8sInfo.K8S_POD_NAME),
			PodNamespace: string(k8sInfo.K8S_POD_NAMESPACE),
		},
	})
	if err != nil {
		logger.Error(err, "failed to send nicDel request")
	}

	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	// get cni config
	config, err := conf.ParseCNIConfig(args.StdinData)
	if err != nil {
		return err
	}

	return types.PrintResult(&type100.Result{}, config.CNIVersion)
}
