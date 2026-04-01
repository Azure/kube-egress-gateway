// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containernetworking/plugins/pkg/ns"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
	"github.com/Azure/kube-egress-gateway/pkg/netlinkwrapper"
	"github.com/Azure/kube-egress-gateway/pkg/netnswrapper"
	"github.com/Azure/kube-egress-gateway/pkg/wgctrlwrapper"
)

var _ reconcile.Reconciler = &NodeReconciler{}

// NodeReconciler watches the current node and manages the LB health probe
// drain state based on the gateway drain label.
type NodeReconciler struct {
	client.Client
	LBProbeServer *healthprobe.LBProbeServer
	Netlink       netlinkwrapper.Interface
	NetNS         netnswrapper.Interface
	WgCtrl        wgctrlwrapper.Interface
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Only care about this node
	if req.Name != os.Getenv(consts.NodeNameEnvKey) {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling node", "name", req.Name)

	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if client.IgnoreNotFound(err) == nil {
			log.Info("Node not found, removing all gateways from health probe")
			r.removeAllGateways()
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if node.Labels[consts.GatewayDrainLabel] == "true" {
		log.Info("Node is marked for drain, removing all gateways from health probe and clearing WireGuard peers")
		r.removeAllGateways()
		if err := r.removeAllWireGuardPeers(ctx); err != nil {
			log.Error(err, "failed to remove WireGuard peers during drain")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) removeAllGateways() {
	for _, gw := range r.LBProbeServer.GetGateways() {
		_ = r.LBProbeServer.RemoveGateway(gw)
	}
}

func (r *NodeReconciler) removeAllWireGuardPeers(ctx context.Context) error {
	log := log.FromContext(ctx)

	gwns, err := r.NetNS.GetNS(consts.GatewayNetnsName)
	if err != nil {
		return fmt.Errorf("failed to get gateway network namespace: %w", err)
	}
	defer func() { _ = gwns.Close() }()

	return gwns.Do(func(_ ns.NetNS) error {
		// List all links to find WireGuard interfaces by name prefix
		links, err := r.Netlink.LinkList()
		if err != nil {
			return fmt.Errorf("failed to list links in gateway namespace: %w", err)
		}

		wgClient, err := r.WgCtrl.New()
		if err != nil {
			return fmt.Errorf("failed to create wgctrl client: %w", err)
		}
		defer func() { _ = wgClient.Close() }()

		for _, link := range links {
			linkName := link.Attrs().Name
			if !strings.HasPrefix(linkName, consts.WiregaurdLinkNamePrefix) {
				continue
			}

			device, err := wgClient.Device(linkName)
			if err != nil {
				log.Error(err, "failed to get WireGuard device", "device", linkName)
				continue
			}

			var peersToRemove []wgtypes.PeerConfig
			for _, peer := range device.Peers {
				peersToRemove = append(peersToRemove, wgtypes.PeerConfig{
					PublicKey: peer.PublicKey,
					Remove:    true,
				})
			}
			if len(peersToRemove) > 0 {
				log.Info("Removing all WireGuard peers", "device", linkName, "count", len(peersToRemove))
				if err := wgClient.ConfigureDevice(linkName, wgtypes.Config{
					Peers: peersToRemove,
				}); err != nil {
					return fmt.Errorf("failed to remove peers from %s: %w", linkName, err)
				}
			}
		}
		return nil
	})
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
