// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

// This file contains a modified version of the StaticGatewayConfigurationReconciler
// updated to use the compatibility layer for Go 1.25.0

package manager

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/compat"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/validator"
)

// StaticGatewayConfigurationReconciler reconciles a StaticGatewayConfiguration object
type StaticGatewayConfigurationReconciler struct {
	client.Client
	CompatClient   *compat.CompatClient // Added compatibility client
	Gateway        string
	NS             string
	VXLANVTEPPORT  int
	VXLANID        int
	Recorder       record.EventRecorder
	NodeSelector   map[string]string
	EnableFirewall bool
	NodeName       string
	GatewayName    string
}

// Reconcile performs the reconciliation logic for StaticGatewayConfiguration
func (r *StaticGatewayConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("static-gateway-reconciler")
	logger.Info("Reconciling StaticGatewayConfiguration", "name", req.Name, "namespace", req.Namespace)

	// Get the StaticGatewayConfiguration resource
	sgConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	// Use the compatibility client instead of direct client
	if err := r.CompatClient.Get(ctx, req.NamespacedName, sgConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("StaticGatewayConfiguration resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get StaticGatewayConfiguration")
		return ctrl.Result{}, err
	}

	// Example of updating a status using the compatibility client
	if err := r.CompatClient.Status().Update(ctx, sgConfig); err != nil {
		logger.Error(err, "Failed to update StaticGatewayConfiguration status")
		return ctrl.Result{}, err
	}

	// The rest of the reconciler code would be updated similarly to use r.CompatClient
	// instead of r.Client directly

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StaticGatewayConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the compatibility client
	r.CompatClient = compat.NewCompatClient(mgr.GetClient())
	
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.StaticGatewayConfiguration{}).
		Complete(r)
}
