// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"os"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/healthprobe"
)

var _ reconcile.Reconciler = &NodeReconciler{}

// NodeReconciler watches the current node and manages the LB health probe
// drain state based on the gateway drain label.
type NodeReconciler struct {
	client.Client
	LBProbeServer *healthprobe.LBProbeServer
}

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Only care about this node
	if req.Name != os.Getenv(consts.NodeNameEnvKey) {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling node %s", req.Name)

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
		log.Info("Node is marked for drain, removing all gateways from health probe")
		r.removeAllGateways()
	}

	return ctrl.Result{}, nil
}

func (r *NodeReconciler) removeAllGateways() {
	for _, gw := range r.LBProbeServer.GetGateways() {
		_ = r.LBProbeServer.RemoveGateway(gw)
	}
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Complete(r)
}
