// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

// This file contains a modified version of the GatewayLBConfigurationReconciler
// updated to use the compatibility layer for Go 1.25.0

package manager

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/compat"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/metrics"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

// GatewayLBConfigurationReconciler reconciles a GatewayLBConfiguration object
type GatewayLBConfigurationReconciler struct {
	client.Client
	CompatClient *compat.CompatClient // Added compatibility client
	*azmanager.AzureManager
	Recorder    record.EventRecorder
	LBProbePort int
}

// Reconcile performs the reconciliation logic for GatewayLBConfiguration
func (r *GatewayLBConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName("lb-reconciler")
	logger.Info("Reconciling GatewayLBConfiguration", "name", req.Name, "namespace", req.Namespace)

	lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
	// Use the compatibility client instead of direct client
	if err := r.CompatClient.Get(ctx, req.NamespacedName, lbConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("GatewayLBConfiguration resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get GatewayLBConfiguration")
		return ctrl.Result{}, err
	}

	gwConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
	if err := r.CompatClient.Get(ctx, req.NamespacedName, gwConfig); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("GatewayLBConfiguration resource not found")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get GatewayLBConfiguration")
		return ctrl.Result{}, err
	}

	// Example of updating a status using the compatibility client
	if err := r.CompatClient.Status().Update(ctx, gwConfig); err != nil {
		logger.Error(err, "Failed to update GatewayLBConfiguration status")
		return ctrl.Result{}, err
	}

	// The rest of the reconciler code would be updated similarly to use r.CompatClient
	// instead of r.Client directly

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayLBConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize the compatibility client
	r.CompatClient = compat.NewCompatClient(mgr.GetClient())
	
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.GatewayLBConfiguration{}).
		Complete(r)
}
