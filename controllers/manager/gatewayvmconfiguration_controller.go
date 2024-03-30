// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/metrics"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

// GatewayVMConfigurationReconciler reconciles a GatewayVMConfiguration object
type GatewayVMConfigurationReconciler struct {
	client.Client
	*azmanager.AzureManager
	Recorder record.EventRecorder
}

var (
	publicIPPrefixRE = regexp.MustCompile(`(?i).*/subscriptions/(.+)/resourceGroups/(.+)/providers/Microsoft.Network/publicIPPrefixes/(.+)`)
)

//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayVMConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *GatewayVMConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// handle node events and enqueue corresponding gatewayVMConfigurations if nodepool matches
	if req.Namespace == "" && req.Name != "" {
		log.Info(fmt.Sprintf("Reconciling node event %s", req.Name))
		node := &corev1.Node{}
		if err := r.Get(ctx, req.NamespacedName, node); err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{}, nil
			}
			log.Error(err, "unable to fetch node instance")
			return ctrl.Result{}, err
		}

		vmConfigList := &egressgatewayv1alpha1.GatewayVMConfigurationList{}
		if err := r.List(ctx, vmConfigList); err != nil {
			log.Error(err, "failed to list GatewayVMConfiguration")
			return ctrl.Result{}, err
		}
		var aggregateError error
		for _, vmConfig := range vmConfigList.Items {
			// skip reconciling when
			// 1. node has agentpool label
			// 2. vmConfig has GatewayNodepoolName and node agentpool label does not match
			// or vmConfig is deleting
			if !vmConfig.ObjectMeta.DeletionTimestamp.IsZero() {
				continue
			}
			if v, ok := node.Labels[consts.AKSNodepoolNameLabel]; ok {
				if npName := vmConfig.Spec.GatewayNodepoolName; npName != "" && !strings.EqualFold(v, npName) {
					continue
				}
			}
			log.Info(fmt.Sprintf("reconcile vmConfig (%s/%s) upon node (%s) event", vmConfig.GetNamespace(), vmConfig.GetName(), req.Name))
			if _, err := r.reconcile(ctx, &vmConfig); err != nil {
				log.Error(err, "failed to reconcile GatewayVMConfiguration")
				aggregateError = errors.Join(aggregateError, err)
				continue // continue to reconcile other vmConfigs
			}
		}
		return ctrl.Result{}, aggregateError
	}

	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, vmConfig); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch GatewayVMConfiguration instance")
		return ctrl.Result{}, err
	}

	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, gwConfig); err != nil {
		log.Error(err, "failed to fetch StaticGatewayConfiguration instance")
		return ctrl.Result{}, err
	}

	if !vmConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Clean up gatewayVMConfiguration
		res, err := r.ensureDeleted(ctx, vmConfig)
		if err != nil {
			r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "EnsureDeleteGatewayVMConfigurationError", err.Error())
		}
		return res, err
	}

	res, err := r.reconcile(ctx, vmConfig)
	if err != nil {
		r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "ReconcileGatewayVMConfigurationError", err.Error())
	} else {
		r.Recorder.Event(gwConfig, corev1.EventTypeNormal, "ReconcileGatewayVMConfigurationSuccess", "GatewayVMConfiguration reconciled")
	}
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayVMConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.GatewayVMConfiguration{}).
		// allow for node events to trigger reconciliation when either node label matches
		Watches(&corev1.Node{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(resourceHasFilterLabel(
			map[string]string{consts.AKSNodepoolModeLabel: consts.AKSNodepoolModeValue, consts.UpstreamNodepoolModeLabel: "true"}))).
		Complete(r)
}

// resourceHasFilterLabel returns a predicate that returns true only if the provided resource contains a label
func resourceHasFilterLabel(m map[string]string) predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return ifLabelMatch(e.ObjectNew, m)
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return ifLabelMatch(e.Object, m)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return ifLabelMatch(e.Object, m)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return ifLabelMatch(e.Object, m)
		},
	}
}

func ifLabelMatch(obj client.Object, m map[string]string) bool {
	if len(m) == 0 {
		return true
	}

	// return true if any label matches
	for labelKey, labelValue := range m {
		if labelKey == "" {
			return true
		}

		lables := obj.GetLabels()
		if v, ok := lables[labelKey]; ok {
			// Return early if no labelValue was set.
			if labelValue == "" {
				return true
			}

			if strings.EqualFold(v, labelValue) {
				return true
			}
		}
	}

	return false
}

func (r *GatewayVMConfigurationReconciler) reconcile(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling GatewayVMConfiguration %s/%s", vmConfig.Namespace, vmConfig.Name))

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"reconcile_gateway_vm_configuration",
		r.SubscriptionID(),
		r.ResourceGroup,
		strings.ToLower(fmt.Sprintf("%s/%s", vmConfig.Namespace, vmConfig.Name)),
	)
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	if !controllerutil.ContainsFinalizer(vmConfig, consts.VMConfigFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(vmConfig, consts.VMConfigFinalizerName)
		err := r.Update(ctx, vmConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	existing := &egressgatewayv1alpha1.GatewayVMConfiguration{}
	vmConfig.DeepCopyInto(existing)

	vmss, ipPrefixLength, err := r.getGatewayVMSS(ctx, vmConfig)
	if err != nil {
		log.Error(err, "failed to get vmss")
		return ctrl.Result{}, err
	}

	ipPrefix, ipPrefixID, isManaged, err := r.ensurePublicIPPrefix(ctx, ipPrefixLength, vmConfig)
	if err != nil {
		log.Error(err, "failed to ensure public ip prefix")
		return ctrl.Result{}, err
	}

	var privateIPs []string
	if privateIPs, err = r.reconcileVMSS(ctx, vmConfig, vmss, ipPrefixID, true); err != nil {
		log.Error(err, "failed to reconcile VMSS")
		return ctrl.Result{}, err
	}

	if !isManaged {
		if err := r.ensurePublicIPPrefixDeleted(ctx, vmConfig); err != nil {
			log.Error(err, "failed to remove managed public ip prefix")
			return ctrl.Result{}, err
		}
	}

	if vmConfig.Status == nil {
		vmConfig.Status = &egressgatewayv1alpha1.GatewayVMConfigurationStatus{}
	}
	if vmConfig.Spec.ProvisionPublicIps {
		vmConfig.Status.EgressIpPrefix = ipPrefix
	} else {
		vmConfig.Status.EgressIpPrefix = strings.Join(privateIPs, ",")
	}

	if !equality.Semantic.DeepEqual(existing, vmConfig) {
		log.Info(fmt.Sprintf("Updating GatewayVMConfiguration %s/%s", vmConfig.Namespace, vmConfig.Name))
		if err := r.Status().Update(ctx, vmConfig); err != nil {
			log.Error(err, "failed to update gateway vm configuration")
		}
	}

	log.Info("GatewayVMConfiguration reconciled")
	succeeded = true
	return ctrl.Result{}, nil
}

func (r *GatewayVMConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling gatewayVMConfiguration deletion %s/%s", vmConfig.Namespace, vmConfig.Name))

	if !controllerutil.ContainsFinalizer(vmConfig, consts.VMConfigFinalizerName) {
		log.Info("vmConfig does not have finalizer, no additional cleanup needed")
		return ctrl.Result{}, nil
	}

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"delete_gateway_vm_configuration",
		r.SubscriptionID(),
		r.ResourceGroup,
		strings.ToLower(fmt.Sprintf("%s/%s", vmConfig.Namespace, vmConfig.Name)),
	)
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	vmss, _, err := r.getGatewayVMSS(ctx, vmConfig)
	if err != nil {
		log.Error(err, "failed to get vmss")
		return ctrl.Result{}, err
	}

	if _, err := r.reconcileVMSS(ctx, vmConfig, vmss, "", false); err != nil {
		log.Error(err, "failed to reconcile VMSS")
		return ctrl.Result{}, err
	}

	if err := r.ensurePublicIPPrefixDeleted(ctx, vmConfig); err != nil {
		log.Error(err, "failed to delete managed public ip prefix")
		return ctrl.Result{}, err
	}

	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(vmConfig, consts.VMConfigFinalizerName)
	if err := r.Update(ctx, vmConfig); err != nil {
		log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("GatewayVMConfiguration deletion reconciled")
	succeeded = true
	return ctrl.Result{}, nil
}

func (r *GatewayVMConfigurationReconciler) getGatewayVMSS(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) (*compute.VirtualMachineScaleSet, int32, error) {
	if vmConfig.Spec.GatewayNodepoolName != "" {
		vmssList, err := r.ListVMSS(ctx)
		if err != nil {
			return nil, 0, err
		}
		for i := range vmssList {
			vmss := vmssList[i]
			if v, ok := vmss.Tags[consts.AKSNodepoolTagKey]; ok {
				if strings.EqualFold(to.Val(v), vmConfig.Spec.GatewayNodepoolName) {
					if prefixLenStr, ok := vmss.Tags[consts.AKSNodepoolIPPrefixSizeTagKey]; ok {
						if prefixLen, err := strconv.Atoi(to.Val(prefixLenStr)); err == nil && prefixLen > 0 && prefixLen <= math.MaxInt32 {
							return vmss, int32(prefixLen), nil
						} else {
							return nil, 0, fmt.Errorf("failed to parse nodepool IP prefix size: %s", to.Val(prefixLenStr))
						}
					} else {
						return nil, 0, fmt.Errorf("nodepool does not have IP prefix size")
					}
				}
			}
		}
	} else {
		vmss, err := r.GetVMSS(ctx, vmConfig.Spec.VmssResourceGroup, vmConfig.Spec.VmssName)
		if err != nil {
			return nil, 0, err
		}
		return vmss, vmConfig.Spec.PublicIpPrefixSize, nil
	}
	return nil, 0, fmt.Errorf("gateway VMSS not found")
}

func managedSubresourceName(vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration) string {
	return consts.ManagedResourcePrefix + string(vmConfig.GetUID())
}

func isErrorNotFound(err error) bool {
	var respErr *azcore.ResponseError
	return errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound
}

func (r *GatewayVMConfigurationReconciler) ensurePublicIPPrefix(
	ctx context.Context,
	ipPrefixLength int32,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) (string, string, bool, error) {
	log := log.FromContext(ctx)

	// no need to provision public ip prefix is only private egress is needed
	if !vmConfig.Spec.ProvisionPublicIps {
		// return isManaged as false so that previously created managed public ip prefix can be deleted
		return "", "", false, nil
	}

	if vmConfig.Spec.PublicIpPrefixId != "" {
		// if there is public prefix ip specified, prioritize this one
		matches := publicIPPrefixRE.FindStringSubmatch(vmConfig.Spec.PublicIpPrefixId)
		if len(matches) != 4 {
			return "", "", false, fmt.Errorf("failed to parse public ip prefix id: %s", vmConfig.Spec.PublicIpPrefixId)
		}
		subscriptionID, resourceGroupName, publicIpPrefixName := matches[1], matches[2], matches[3]
		if subscriptionID != r.SubscriptionID() {
			return "", "", false, fmt.Errorf("public ip prefix subscription(%s) is not in the same subscription(%s)", subscriptionID, r.SubscriptionID())
		}
		ipPrefix, err := r.GetPublicIPPrefix(ctx, resourceGroupName, publicIpPrefixName)
		if err != nil {
			return "", "", false, fmt.Errorf("failed to get public ip prefix(%s): %w", vmConfig.Spec.PublicIpPrefixId, err)
		}
		if ipPrefix.Properties == nil {
			return "", "", false, fmt.Errorf("public ip prefix(%s) has empty properties", vmConfig.Spec.PublicIpPrefixId)
		}
		if to.Val(ipPrefix.Properties.PrefixLength) != ipPrefixLength {
			return "", "", false, fmt.Errorf("provided public ip prefix has invalid length(%d), required(%d)", to.Val(ipPrefix.Properties.PrefixLength), ipPrefixLength)
		}
		log.Info("Found existing unmanaged public ip prefix", "public ip prefix", to.Val(ipPrefix.Properties.IPPrefix))
		return to.Val(ipPrefix.Properties.IPPrefix), to.Val(ipPrefix.ID), false, nil
	} else {
		// check if there's managed public prefix ip
		publicIpPrefixName := managedSubresourceName(vmConfig)
		ipPrefix, err := r.GetPublicIPPrefix(ctx, "", publicIpPrefixName)
		if err == nil {
			if ipPrefix.Properties == nil {
				return "", "", false, fmt.Errorf("managed public ip prefix has empty properties")
			} else {
				log.Info("Found existing managed public ip prefix", "public ip prefix", to.Val(ipPrefix.Properties.IPPrefix))
				return to.Val(ipPrefix.Properties.IPPrefix), to.Val(ipPrefix.ID), true, nil
			}
		} else {
			if !isErrorNotFound(err) {
				return "", "", false, fmt.Errorf("failed to get managed public ip prefix: %w", err)
			}
			// create new public ip prefix
			newIPPrefix := network.PublicIPPrefix{
				Name:     to.Ptr(publicIpPrefixName),
				Location: to.Ptr(r.Location()),
				Properties: &network.PublicIPPrefixPropertiesFormat{
					PrefixLength:           to.Ptr(ipPrefixLength),
					PublicIPAddressVersion: to.Ptr(network.IPVersionIPv4),
				},
				SKU: &network.PublicIPPrefixSKU{
					Name: to.Ptr(network.PublicIPPrefixSKUNameStandard),
					Tier: to.Ptr(network.PublicIPPrefixSKUTierRegional),
				},
			}
			log.Info("Creating new managed public ip prefix")
			ipPrefix, err := r.CreateOrUpdatePublicIPPrefix(ctx, "", publicIpPrefixName, newIPPrefix)
			if err != nil {
				return "", "", false, fmt.Errorf("failed to create managed public ip prefix: %w", err)
			}
			return to.Val(ipPrefix.Properties.IPPrefix), to.Val(ipPrefix.ID), true, nil
		}
	}
}

func (r *GatewayVMConfigurationReconciler) ensurePublicIPPrefixDeleted(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
) error {
	log := log.FromContext(ctx)
	// only ensure managed public prefix ip is deleted
	publicIpPrefixName := managedSubresourceName(vmConfig)
	_, err := r.GetPublicIPPrefix(ctx, "", publicIpPrefixName)
	if err != nil {
		if isErrorNotFound(err) {
			// resource does not exist, directly return
			return nil
		} else {
			return fmt.Errorf("failed to get public ip prefix(%s): %w", publicIpPrefixName, err)
		}
	} else {
		log.Info("Deleting managed public ip prefix", "public ip prefix name", publicIpPrefixName)
		if err := r.DeletePublicIPPrefix(ctx, "", publicIpPrefixName); err != nil {
			return fmt.Errorf("failed to delete public ip prefix(%s): %w", publicIpPrefixName, err)
		}
		return nil
	}
}

func (r *GatewayVMConfigurationReconciler) reconcileVMSS(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
	vmss *compute.VirtualMachineScaleSet,
	ipPrefixID string,
	wantIPConfig bool,
) ([]string, error) {
	log := log.FromContext(ctx)
	ipConfigName := managedSubresourceName(vmConfig)
	needUpdate := false

	if vmss.Properties == nil || vmss.Properties.VirtualMachineProfile == nil ||
		vmss.Properties.VirtualMachineProfile.NetworkProfile == nil {
		return nil, fmt.Errorf("vmss has empty network profile")
	}

	lbBackendpoolID := r.GetLBBackendAddressPoolID(to.Val(vmss.Properties.UniqueID))
	interfaces := vmss.Properties.VirtualMachineProfile.NetworkProfile.NetworkInterfaceConfigurations
	needUpdate, err := r.reconcileVMSSNetworkInterface(ctx, ipConfigName, ipPrefixID, to.Val(lbBackendpoolID), wantIPConfig, interfaces)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile vmss interface(%s): %w", to.Val(vmss.Name), err)
	}

	if needUpdate {
		log.Info("Updating vmss", "vmssName", to.Val(vmss.Name))
		newVmss := compute.VirtualMachineScaleSet{
			Location: vmss.Location,
			Properties: &compute.VirtualMachineScaleSetProperties{
				VirtualMachineProfile: &compute.VirtualMachineScaleSetVMProfile{
					NetworkProfile: vmss.Properties.VirtualMachineProfile.NetworkProfile,
				},
			},
		}
		if _, err := r.CreateOrUpdateVMSS(ctx, "", to.Val(vmss.Name), newVmss); err != nil {
			return nil, fmt.Errorf("failed to update vmss(%s): %w", to.Val(vmss.Name), err)
		}
	}

	// check and update VMSS instances
	var privateIPs []string
	instances, err := r.ListVMSSInstances(ctx, "", to.Val(vmss.Name))
	if err != nil {
		return nil, fmt.Errorf("failed to get vm instances from vmss(%s): %w", to.Val(vmss.Name), err)
	}
	for _, instance := range instances {
		privateIP, err := r.reconcileVMSSVM(ctx, vmConfig, to.Val(vmss.Name), instance, ipPrefixID, to.Val(lbBackendpoolID), wantIPConfig)
		if err != nil {
			return nil, err
		}
		if wantIPConfig && ipPrefixID == "" {
			privateIPs = append(privateIPs, privateIP)
		}
	}
	// clean up VMProfiles for deleted nodes
	var vmprofiles []egressgatewayv1alpha1.GatewayVMProfile
	if vmConfig.Status != nil {
		for i := range vmConfig.Status.GatewayVMProfiles {
			profile := vmConfig.Status.GatewayVMProfiles[i]
			for _, instance := range instances {
				if profile.NodeName == to.Val(instance.Properties.OSProfile.ComputerName) {
					vmprofiles = append(vmprofiles, profile)
					break
				}
			}
		}
		vmConfig.Status.GatewayVMProfiles = vmprofiles
	}

	err = r.Status().Update(ctx, vmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to update vm config status: %w", err)
	}

	return privateIPs, nil
}

func (r *GatewayVMConfigurationReconciler) reconcileVMSSVM(
	ctx context.Context,
	vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
	vmssName string,
	vm *compute.VirtualMachineScaleSetVM,
	ipPrefixID string,
	lbBackendpoolID string,
	wantIPConfig bool,
) (string, error) {
	log := log.FromContext(ctx)
	ipConfigName := managedSubresourceName(vmConfig)

	if vm.Properties == nil || vm.Properties.NetworkProfileConfiguration == nil {
		return "", fmt.Errorf("vmss vm(%s) has empty network profile", to.Val(vm.InstanceID))
	}
	if vm.Properties.OSProfile == nil {
		return "", fmt.Errorf("vmss vm(%s) has empty os profile", to.Val(vm.InstanceID))
	}

	interfaces := vm.Properties.NetworkProfileConfiguration.NetworkInterfaceConfigurations
	needUpdate, err := r.reconcileVMSSNetworkInterface(ctx, ipConfigName, ipPrefixID, lbBackendpoolID, wantIPConfig, interfaces)
	if err != nil {
		return "", fmt.Errorf("failed to reconcile vm interface(%s): %w", to.Val(vm.InstanceID), err)
	}
	if needUpdate {
		log.Info("Updating vmss instance", "vmInstanceID", to.Val(vm.InstanceID))
		newVM := compute.VirtualMachineScaleSetVM{
			Properties: &compute.VirtualMachineScaleSetVMProperties{
				NetworkProfileConfiguration: &compute.VirtualMachineScaleSetVMNetworkProfileConfiguration{
					NetworkInterfaceConfigurations: interfaces,
				},
			},
		}
		if _, err := r.UpdateVMSSInstance(ctx, "", vmssName, to.Val(vm.InstanceID), newVM); err != nil {
			return "", fmt.Errorf("failed to update vmss instance(%s): %w", to.Val(vm.InstanceID), err)
		}
	}

	// return earlier if it's deleting event
	if !wantIPConfig {
		return "", nil
	}

	var primaryIP, secondaryIP string
	for _, nic := range interfaces {
		if nic.Properties != nil && to.Val(nic.Properties.Primary) {
			vmNic, err := r.GetVMSSInterface(ctx, "", vmssName, to.Val(vm.InstanceID), to.Val(nic.Name))
			if err != nil {
				return "", fmt.Errorf("failed to get vmss(%s) instance(%s) nic(%s): %w", vmssName, to.Val(vm.InstanceID), to.Val(nic.Name), err)
			}
			if vmNic.Properties == nil || vmNic.Properties.IPConfigurations == nil {
				return "", fmt.Errorf("vmss(%s) instance(%s) nic(%s) has empty ip configurations", vmssName, to.Val(vm.InstanceID), to.Val(nic.Name))
			}
			for _, ipConfig := range vmNic.Properties.IPConfigurations {
				if ipConfig != nil && ipConfig.Properties != nil && strings.EqualFold(to.Val(ipConfig.Name), ipConfigName) {
					secondaryIP = to.Val(ipConfig.Properties.PrivateIPAddress)
				} else if ipConfig != nil && ipConfig.Properties != nil && to.Val(ipConfig.Properties.Primary) {
					primaryIP = to.Val(ipConfig.Properties.PrivateIPAddress)
				}
			}
		}
	}
	if primaryIP == "" || secondaryIP == "" {
		return "", fmt.Errorf("failed to find private IP from vmss(%s), instance(%s), ipConfig(%s)", vmssName, to.Val(vm.InstanceID), ipConfigName)
	}

	vmprofile := egressgatewayv1alpha1.GatewayVMProfile{
		NodeName:    to.Val(vm.Properties.OSProfile.ComputerName),
		PrimaryIP:   primaryIP,
		SecondaryIP: secondaryIP,
	}
	if vmConfig.Status == nil {
		vmConfig.Status = &egressgatewayv1alpha1.GatewayVMConfigurationStatus{}
	}
	for i, profile := range vmConfig.Status.GatewayVMProfiles {
		if profile.NodeName == vmprofile.NodeName {
			if profile.PrimaryIP != primaryIP || profile.SecondaryIP != secondaryIP {
				vmConfig.Status.GatewayVMProfiles[i].PrimaryIP = primaryIP
				vmConfig.Status.GatewayVMProfiles[i].SecondaryIP = secondaryIP
				log.Info("GatewayVMConfiguration status updated", "primaryIP", primaryIP, "secondaryIP", secondaryIP)
				return secondaryIP, nil
			}
			log.Info("GatewayVMConfiguration status not changed", "primaryIP", primaryIP, "secondaryIP", secondaryIP)
			return secondaryIP, nil
		}
	}

	log.Info("GatewayVMConfiguration status updated for new nodes", "nodeName", vmprofile.NodeName, "primaryIP", primaryIP, "secondaryIP", secondaryIP)
	vmConfig.Status.GatewayVMProfiles = append(vmConfig.Status.GatewayVMProfiles, vmprofile)

	return secondaryIP, nil
}

func (r *GatewayVMConfigurationReconciler) reconcileVMSSNetworkInterface(
	ctx context.Context,
	ipConfigName string,
	ipPrefixID string,
	lbBackendpoolID string,
	wantIPConfig bool,
	interfaces []*compute.VirtualMachineScaleSetNetworkConfiguration,
) (bool, error) {
	log := log.FromContext(ctx)
	expectedConfig := r.getExpectedIPConfig(ipConfigName, ipPrefixID, interfaces)
	var primaryNic *compute.VirtualMachineScaleSetNetworkConfiguration
	needUpdate := false
	foundConfig := false

	for _, nic := range interfaces {
		if nic.Properties != nil && to.Val(nic.Properties.Primary) {
			primaryNic = nic
			for i, ipConfig := range nic.Properties.IPConfigurations {
				if to.Val(ipConfig.Name) == ipConfigName {
					if !wantIPConfig {
						log.Info("Found unwanted ipConfig, dropping")
						nic.Properties.IPConfigurations = append(nic.Properties.IPConfigurations[:i], nic.Properties.IPConfigurations[i+1:]...)
						needUpdate = true
					} else {
						if different(ipConfig, expectedConfig) {
							log.Info("Found target ipConfig with different configurations, dropping")
							needUpdate = true
							nic.Properties.IPConfigurations = append(nic.Properties.IPConfigurations[:i], nic.Properties.IPConfigurations[i+1:]...)
						} else {
							log.Info("Found expected ipConfig, keeping")
							foundConfig = true
						}
					}
					break
				}
			}
		}
	}

	if wantIPConfig && !foundConfig {
		if primaryNic == nil {
			return false, fmt.Errorf("vmss(vm) primary network interface not found")
		}
		primaryNic.Properties.IPConfigurations = append(primaryNic.Properties.IPConfigurations, expectedConfig)
		needUpdate = true
	}

	changed, err := r.reconcileLbBackendPool(lbBackendpoolID, primaryNic)
	if err != nil {
		return false, err
	}
	needUpdate = needUpdate || changed

	return needUpdate, nil
}

func (r *GatewayVMConfigurationReconciler) reconcileLbBackendPool(
	lbBackendpoolID string,
	primaryNic *compute.VirtualMachineScaleSetNetworkConfiguration,
) (needUpdate bool, err error) {
	if primaryNic == nil {
		return false, fmt.Errorf("vmss(vm) primary network interface not found")
	}

	needBackendPool := false
	for _, ipConfig := range primaryNic.Properties.IPConfigurations {
		if strings.HasPrefix(to.Val(ipConfig.Name), consts.ManagedResourcePrefix) {
			needBackendPool = true
			break
		}
	}

	for _, ipConfig := range primaryNic.Properties.IPConfigurations {
		if ipConfig.Properties != nil && to.Val(ipConfig.Properties.Primary) {
			backendPools := ipConfig.Properties.LoadBalancerBackendAddressPools
			for i, backend := range backendPools {
				if strings.EqualFold(lbBackendpoolID, to.Val(backend.ID)) {
					if !needBackendPool {
						backendPools = append(backendPools[:i], backendPools[i+1:]...)
						ipConfig.Properties.LoadBalancerBackendAddressPools = backendPools
						return true, nil
					} else {
						return false, nil
					}
				}
			}
			if !needBackendPool {
				return false, nil
			}
			backendPools = append(backendPools, &compute.SubResource{ID: to.Ptr(lbBackendpoolID)})
			ipConfig.Properties.LoadBalancerBackendAddressPools = backendPools
			return true, nil
		}
	}
	return false, fmt.Errorf("vmss(vm) primary ipConfig not found")
}

func (r *GatewayVMConfigurationReconciler) getExpectedIPConfig(
	ipConfigName,
	ipPrefixID string,
	interfaces []*compute.VirtualMachineScaleSetNetworkConfiguration,
) *compute.VirtualMachineScaleSetIPConfiguration {
	var subnetID *string
	for _, nic := range interfaces {
		if nic.Properties != nil && to.Val(nic.Properties.Primary) {
			for _, ipConfig := range nic.Properties.IPConfigurations {
				if ipConfig.Properties != nil && to.Val(ipConfig.Properties.Primary) {
					subnetID = ipConfig.Properties.Subnet.ID
				}
			}
		}
	}

	var pipConfig *compute.VirtualMachineScaleSetPublicIPAddressConfiguration
	if ipPrefixID != "" {
		pipConfig = &compute.VirtualMachineScaleSetPublicIPAddressConfiguration{
			Name: to.Ptr(ipConfigName),
			Properties: &compute.VirtualMachineScaleSetPublicIPAddressConfigurationProperties{
				PublicIPPrefix: &compute.SubResource{
					ID: to.Ptr(ipPrefixID),
				},
			},
		}
	}
	return &compute.VirtualMachineScaleSetIPConfiguration{
		Name: to.Ptr(ipConfigName),
		Properties: &compute.VirtualMachineScaleSetIPConfigurationProperties{
			Primary:                      to.Ptr(false),
			PrivateIPAddressVersion:      to.Ptr(compute.IPVersionIPv4),
			PublicIPAddressConfiguration: pipConfig,
			Subnet: &compute.APIEntityReference{
				ID: subnetID,
			},
		},
	}
}

func different(ipConfig1, ipConfig2 *compute.VirtualMachineScaleSetIPConfiguration) bool {
	if ipConfig1.Properties == nil && ipConfig2.Properties == nil {
		return false
	}
	if ipConfig1.Properties == nil || ipConfig2.Properties == nil {
		return true
	}
	prop1, prop2 := ipConfig1.Properties, ipConfig2.Properties
	if to.Val(prop1.Primary) != to.Val(prop2.Primary) ||
		to.Val(prop1.PrivateIPAddressVersion) != to.Val(prop2.PrivateIPAddressVersion) {
		return true
	}

	if (prop1.Subnet != nil) != (prop2.Subnet != nil) {
		return true
	} else if prop1.Subnet != nil && prop2.Subnet != nil && !strings.EqualFold(to.Val(prop1.Subnet.ID), to.Val(prop2.Subnet.ID)) {
		return true
	}

	pip1, pip2 := prop1.PublicIPAddressConfiguration, prop2.PublicIPAddressConfiguration
	if (pip1 == nil) != (pip2 == nil) {
		return true
	} else if pip1 != nil && pip2 != nil {
		if to.Val(pip1.Name) != to.Val(pip2.Name) {
			return true
		} else if (pip1.Properties != nil) != (pip2.Properties != nil) {
			return true
		} else if pip1.Properties != nil && pip2.Properties != nil {
			prefix1, prefix2 := pip1.Properties.PublicIPPrefix, pip2.Properties.PublicIPPrefix
			if (prefix1 != nil) != (prefix2 != nil) {
				return true
			} else if prefix1 != nil && prefix2 != nil && !strings.EqualFold(to.Val(prefix1.ID), to.Val(prefix2.ID)) {
				return true
			}
		}
	}
	return false
}
