// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

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
	"github.com/google/uuid"
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
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/metrics"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

var (
	namespaceAgentPool = uuid.Must(uuid.Parse("2c96e82c-842f-11f0-8ea5-6bee14278ecd"))
)

// GatewayLBConfigurationReconciler reconciles a GatewayLBConfiguration object
type GatewayLBConfigurationReconciler struct {
	client.Client
	*azmanager.AzureManager
	Recorder    record.EventRecorder
	LBProbePort int
}

type lbPropertyNames struct {
	frontendName string
	backendName  string
	lbRuleName   string
	probeName    string
}

// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/finalizers,verbs=update
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewayvmconfigurations/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayLBConfiguration object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *GatewayLBConfigurationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the GatewayLBConfiguration instance.
	lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, lbConfig); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch GatewayLBConfiguration instance")
		return ctrl.Result{}, err
	}

	gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
	if err := r.Get(ctx, req.NamespacedName, gwConfig); err != nil {
		log.Error(err, "unable to fetch StaticGatewayConfiguration instance")
		return ctrl.Result{}, err
	}

	if !lbConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Clean up gatewayLBConfiguration
		res, err := r.ensureDeleted(ctx, lbConfig)
		if err != nil {
			r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "EnsureDeleteGatewayLBConfigurationError", err.Error())
		}
		return res, err
	}

	res, err := r.reconcile(ctx, lbConfig)
	if err != nil {
		r.Recorder.Event(gwConfig, corev1.EventTypeWarning, "ReconcileGatewayLBConfigurationError", err.Error())
	} else {
		r.Recorder.Event(gwConfig, corev1.EventTypeNormal, "ReconcileGatewayLBConfigurationSuccess", "GatewayLBConfiguration reconciled")
	}
	return res, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayLBConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.GatewayLBConfiguration{}).
		Owns(&egressgatewayv1alpha1.GatewayVMConfiguration{}).
		Complete(r)
}

func (r *GatewayLBConfigurationReconciler) reconcile(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling GatewayLBConfiguration %s/%s", lbConfig.Namespace, lbConfig.Name))

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"reconcile_gateway_lb_configuration",
		r.SubscriptionID(),
		r.LoadBalancerResourceGroup,
		strings.ToLower(fmt.Sprintf("%s/%s", lbConfig.Namespace, lbConfig.Name)),
	)
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	if !controllerutil.ContainsFinalizer(lbConfig, consts.LBConfigFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
		err := r.Update(ctx, lbConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	existing := &egressgatewayv1alpha1.GatewayLBConfiguration{}
	lbConfig.DeepCopyInto(existing)

	// reconcile LB rule
	ip, port, err := r.reconcileLBRule(ctx, lbConfig, true)
	if err != nil {
		log.Error(err, "failed to reconcile LB rules")
		return ctrl.Result{}, err
	}

	// reconcile vmconfig
	if err := r.reconcileGatewayVMConfig(ctx, lbConfig); err != nil {
		log.Error(err, "failed to reconcile gateway VM configuration")
		return ctrl.Result{}, err
	}

	if lbConfig.Status == nil {
		lbConfig.Status = &egressgatewayv1alpha1.GatewayLBConfigurationStatus{}
	}
	lbConfig.Status.FrontendIp = ip
	lbConfig.Status.ServerPort = port

	if !equality.Semantic.DeepEqual(existing, lbConfig) {
		log.Info(fmt.Sprintf("Updating GatewayLBConfiguration %s/%s", lbConfig.Namespace, lbConfig.Name))
		if err := r.Status().Update(ctx, lbConfig); err != nil {
			log.Error(err, "failed to update gateway LB configuration")
		}
	}

	log.Info("GatewayLBConfiguration reconciled")
	succeeded = true
	return ctrl.Result{}, nil
}

func (r *GatewayLBConfigurationReconciler) ensureDeleted(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling gatewayLBConfiguration deletion %s/%s", lbConfig.Namespace, lbConfig.Name))

	if !controllerutil.ContainsFinalizer(lbConfig, consts.LBConfigFinalizerName) {
		log.Info("lbConfig does not have finalizer, no additional cleanup needed")
		return ctrl.Result{}, nil
	}

	mc := metrics.NewMetricsContext(
		os.Getenv(consts.PodNamespaceEnvKey),
		"delete_gateway_lb_configuration",
		r.SubscriptionID(),
		r.LoadBalancerResourceGroup,
		strings.ToLower(fmt.Sprintf("%s/%s", lbConfig.Namespace, lbConfig.Name)),
	)
	succeeded := false
	defer func() { mc.ObserveControllerReconcileMetrics(succeeded) }()

	log.Info("Deleting VMConfig")
	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lbConfig.Name,
			Namespace: lbConfig.Namespace,
		},
	}
	if err := r.Delete(ctx, vmConfig); err == nil {
		// deleting vmConfig, skip reconciling lb
		log.Info("Waiting gateway vmss to be cleaned first")
		succeeded = true
		return ctrl.Result{}, nil
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "failed to delete existing gateway VM configuration")
		return ctrl.Result{}, err
	} // vmConfig is already deleted, continue to clean up lb

	// delete LB rule
	_, _, err := r.reconcileLBRule(ctx, lbConfig, false)
	if err != nil {
		log.Error(err, "failed to reconcile LB rules")
		return ctrl.Result{}, err
	}

	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(lbConfig, consts.LBConfigFinalizerName)
	if err := r.Update(ctx, lbConfig); err != nil {
		log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("GatewayLBConfiguration deletion reconciled")
	succeeded = true
	return ctrl.Result{}, nil
}

type AgentPool interface {
	Reconcile(ctx context.Context,
		vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration,
		ipPrefixID string,
		wantIPConfig bool) ([]string, error) // todo refactor to some config struct
	GetUniqueID() string
}

func getLBPropertyName(
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
	ap AgentPool,
) (*lbPropertyNames, error) {
	if ap.GetUniqueID() == "" {
		return nil, fmt.Errorf("gateway node pool does not have UID")
	}
	names := &lbPropertyNames{
		frontendName: ap.GetUniqueID(),
		backendName:  ap.GetUniqueID(),
		lbRuleName:   string(lbConfig.GetUID()),
		probeName:    string(lbConfig.GetUID()),
	}
	return names, nil
}

func (r *GatewayLBConfigurationReconciler) getGatewayLB(ctx context.Context) (*network.LoadBalancer, error) {
	lb, err := r.GetLB(ctx)
	if err == nil {
		return lb, nil
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	return nil, err
}

func (r *GatewayLBConfigurationReconciler) getGatewayVMSS(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) (AgentPool, error) {
	if lbConfig.Spec.GatewayNodepoolName != "" {
		vmssList, err := r.ListVMSS(ctx)
		if err != nil {
			return nil, err
		}
		for i := range vmssList {
			vmss := vmssList[i]
			if v, ok := vmss.Tags[consts.AKSNodepoolTagKey]; ok {
				if strings.EqualFold(to.Val(v), lbConfig.Spec.GatewayNodepoolName) {
					return &agentPoolVMSS{vmss: vmss}, nil
				}
			}
		}

		vmsList, err := r.ListVMs(ctx) // this will be expensive, can we page here?
		if err != nil {
			return nil, err
		}
		for i := range vmsList {
			vm := vmsList[i]
			if v, ok := vm.Tags[consts.AKSNodepoolTagKey]; ok {
				if strings.EqualFold(to.Val(v), lbConfig.Spec.GatewayNodepoolName) {
					return &agentPoolVMs{
						agentPoolName: lbConfig.Spec.GatewayNodepoolName,
					}, nil
				}
			}
		}
	} else {
		vmss, err := r.GetVMSS(ctx, lbConfig.Spec.VmssResourceGroup, lbConfig.Spec.VmssName)
		if err != nil {
			return nil, err
		}
		return &agentPoolVMSS{vmss: vmss}, nil
	}
	return nil, fmt.Errorf("gateway VMSS not found")
}

func NewAgentPoolVM(agentPoolName string, c client.Client, manager *azmanager.AzureManager) *agentPoolVMs {
	return &agentPoolVMs{
		agentPoolName: agentPoolName,
		Client:        c,
		AzureManager:  manager,
	}
}

type agentPoolVMs struct {
	agentPoolName string
	client.Client
	*azmanager.AzureManager
}

func (a *agentPoolVMs) Reconcile(ctx context.Context, vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration, ipPrefixID string, wantIPConfig bool) ([]string, error) {
	backendLBPoolID := a.GetLBBackendAddressPoolID(a.GetUniqueID())

	secondaryIPs := make([]string, 0)

	nics, err := a.ListNetworkInterfaces(ctx, "" /* empty resource group, just use default */)
	if err != nil {
		return nil, err
	}

	gatewayNICs := make([]*network.Interface, 0, len(nics))
	for i := range nics {
		if nics[i] == nil {
			continue
		}

		if _, ok := nics[i].Tags["static-gateway-nic"]; !ok {
			continue
		}
		gatewayNICs = append(gatewayNICs, nics[i])
	}

	for i := range gatewayNICs {
		ip, err := a.reconcileNIC(ctx, vmConfig, nics[i], ipPrefixID, to.Val(backendLBPoolID), wantIPConfig)
		if err != nil {
			return nil, err
		}
		secondaryIPs = append(secondaryIPs, ip)
	}
	return secondaryIPs, nil
}

func (a *agentPoolVMs) GetUniqueID() string {
	return uuid.NewMD5(namespaceAgentPool, []byte(a.agentPoolName)).String()
}

func NewAgentPoolVMSS(vmss *compute.VirtualMachineScaleSet, c client.Client, manager *azmanager.AzureManager) *agentPoolVMSS {
	return &agentPoolVMSS{
		vmss:         vmss,
		Client:       c,
		AzureManager: manager,
	}
}

type agentPoolVMSS struct {
	vmss *compute.VirtualMachineScaleSet
	client.Client
	*azmanager.AzureManager
}

func (r *agentPoolVMSS) Reconcile(ctx context.Context, vmConfig *egressgatewayv1alpha1.GatewayVMConfiguration, ipPrefixID string, wantIPConfig bool) ([]string, error) {
	return r.reconcileVMSS(ctx, vmConfig, r.vmss, ipPrefixID, wantIPConfig)
}

func (r *agentPoolVMSS) GetUniqueID() string {
	if r.vmss == nil || r.vmss.Properties == nil || r.vmss.Properties.UniqueID == nil {
		return ""
	}
	return *r.vmss.Properties.UniqueID
}

func (r *GatewayLBConfigurationReconciler) reconcileLBRule(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
	needLB bool,
) (string, int32, error) {
	log := log.FromContext(ctx)
	frontendIP := ""
	var lbPort int32
	updateLB := false
	deleteFrontend := false

	// get LoadBalancer
	lb, err := r.getGatewayLB(ctx)
	if err != nil {
		log.Error(err, "failed to get LoadBalancer")
		return "", 0, err
	}
	if lb == nil {
		if !needLB {
			log.Info(fmt.Sprintf("gateway lb(%s) not found, no more clean up needed", r.LoadBalancerName()))
			return "", 0, nil
		} else {
			lb = &network.LoadBalancer{
				Name:     to.Ptr(r.LoadBalancerName()),
				Location: to.Ptr(r.Location()),
				SKU: &network.LoadBalancerSKU{
					Name: to.Ptr(network.LoadBalancerSKUNameStandard),
					Tier: to.Ptr(network.LoadBalancerSKUTierRegional),
				},
				Properties: &network.LoadBalancerPropertiesFormat{},
			}
			updateLB = true
		}
	}

	// get gateway VMSS
	// we need this because each gateway vmss needs one frontendConfig and one backendpool
	agentPool, err := r.getGatewayVMSS(ctx, lbConfig)
	if err != nil {
		log.Error(err, "failed to get vmss")
		return "", 0, err
	}

	// get lbPropertyNames
	names, err := getLBPropertyName(lbConfig, agentPool)
	if err != nil {
		log.Error(err, "failed to get LB property names")
		return "", 0, err
	}

	if lb.Properties == nil {
		return "", 0, fmt.Errorf("lb property is empty")
	}

	frontendID := r.GetLBFrontendIPConfigurationID(names.frontendName)
	frontendIP, err = findFrontendIP(lb, names.frontendName)
	if err != nil {
		return "", 0, err
	}
	if frontendIP == "" {
		if needLB {
			subnet, err := r.GetSubnet(ctx)
			if err != nil {
				log.Error(err, "failed to get subnet")
				return "", 0, err
			}
			lb.Properties.FrontendIPConfigurations =
				append(lb.Properties.FrontendIPConfigurations, getExpectedFrontendConfig(to.Ptr(names.frontendName), subnet.ID))
			updateLB = true
		}
	} else {
		log.Info("Found LB frontendIPConfiguration", "frontendIP", frontendIP)
	}

	backendID := r.GetLBBackendAddressPoolID(names.backendName)
	foundBackend := false
	for _, backendPool := range lb.Properties.BackendAddressPools {
		if strings.EqualFold(*backendPool.Name, names.backendName) &&
			strings.EqualFold(*backendPool.ID, *backendID) {
			log.Info("Found LB backendAddressPool", "backendName", names.backendName)
			foundBackend = true
			break
		}
	}
	if !foundBackend {
		if needLB {
			lb.Properties.BackendAddressPools =
				append(lb.Properties.BackendAddressPools, getExpectedBackendPool(to.Ptr(names.backendName)))
			updateLB = true
		}
	}

	probeID := r.GetLBProbeID(names.probeName)
	expectedLBRule := getExpectedLBRule(&names.lbRuleName, frontendID, backendID, probeID)
	expectedProbe := getExpectedLBProbe(&names.probeName, r.LBProbePort, lbConfig)

	lbRules := lb.Properties.LoadBalancingRules
	if needLB {
		foundRule := false
		for i := range lbRules {
			lbRule := lbRules[i]
			if strings.EqualFold(*lbRule.Name, *expectedLBRule.Name) {
				if lbRule.Properties == nil {
					log.Info("Found LB rule with empty properties, dropping")
					lbRules = append(lbRules[:i], lbRules[i+1:]...)
				} else if !sameLBRuleConfig(ctx, lbRule, expectedLBRule) {
					log.Info("Found LB rule with different configuration, dropping")
					lbRules = append(lbRules[:i], lbRules[i+1:]...)
				} else {
					log.Info("Found expected LB rule, keeping")
					foundRule = true
					lbPort = to.Val(lbRule.Properties.FrontendPort)
				}
				break
			}
		}
		if !foundRule {
			port, err := selectPortForLBRule(expectedLBRule, lbRules)
			if err != nil {
				return "", 0, err
			}
			log.Info("Creating new lbRule", "port", port)
			lbPort = port
			expectedLBRule.Properties.FrontendPort = &port
			expectedLBRule.Properties.BackendPort = &port
			lbRules = append(lbRules, expectedLBRule)
			lb.Properties.LoadBalancingRules = lbRules
			updateLB = true
		}
	} else {
		ruleRefCnt := 0
		for i := len(lbRules) - 1; i >= 0; i = i - 1 {
			lbRule := lbRules[i]
			if strings.EqualFold(*lbRule.Name, *expectedLBRule.Name) {
				log.Info("Found LB rule, dropping")
				lbRules = append(lbRules[:i], lbRules[i+1:]...)
				updateLB = true
				lb.Properties.LoadBalancingRules = lbRules
			} else if strings.EqualFold(to.Val(lbRule.Properties.FrontendIPConfiguration.ID), to.Val(frontendID)) {
				ruleRefCnt = ruleRefCnt + 1
			}
		}
		if ruleRefCnt == 0 {
			deleteFrontend = true
		}
	}

	probes := lb.Properties.Probes
	if needLB {
		foundProbe := false
		for i := range probes {
			probe := probes[i]
			if strings.EqualFold(*probe.Name, *expectedProbe.Name) {
				if probe.Properties == nil {
					log.Info("Found LB probe with empty properties, dropping")
					probes = append(probes[:i], probes[i+1:]...)
				}
				if to.Val(probe.Properties.RequestPath) != to.Val(expectedProbe.Properties.RequestPath) ||
					to.Val(probe.Properties.Port) != to.Val(expectedProbe.Properties.Port) ||
					*probe.Properties.Protocol != *expectedProbe.Properties.Protocol {
					log.Info("Found LB probe with different configuration, dropping")
					probes = append(probes[:i], probes[i+1:]...)
				} else {
					log.Info("Found expected LB probe, keeping")
					foundProbe = true
				}
				break
			}
		}
		if !foundProbe {
			log.Info("Creating new probe")
			probes = append(probes, expectedProbe)
			lb.Properties.Probes = probes
			updateLB = true
		}
	} else {
		for i := range probes {
			probe := probes[i]
			if strings.EqualFold(*probe.Name, *expectedProbe.Name) {
				log.Info("Found LB probe, dropping")
				probes = append(probes[:i], probes[i+1:]...)
				updateLB = true
				lb.Properties.Probes = probes
				break
			}
		}
	}

	if !needLB && deleteFrontend {
		log.Info(fmt.Sprintf("no more gateway profile referring vmss(%s), deleting frontend and backend", names.frontendName))
		frontends := lb.Properties.FrontendIPConfigurations
		for i, frontendConfig := range frontends {
			if strings.EqualFold(to.Val(frontendConfig.ID), to.Val(frontendID)) {
				frontends = append(frontends[:i], frontends[i+1:]...)
				updateLB = true
				lb.Properties.FrontendIPConfigurations = frontends
				break
			}
		}
		backends := lb.Properties.BackendAddressPools
		for i, backendPool := range backends {
			if strings.EqualFold(to.Val(backendPool.ID), to.Val(backendID)) {
				backends = append(backends[:i], backends[i+1:]...)
				updateLB = true
				lb.Properties.BackendAddressPools = backends
				break
			}
		}

		if len(lb.Properties.FrontendIPConfigurations) == 0 {
			log.Info("Deleting load balancer")
			if err := r.DeleteLB(ctx); err != nil {
				log.Error(err, "failed to delete LB")
				return "", 0, err
			}
			return "", 0, nil
		}
	}

	if updateLB {
		log.Info("Updating load balancer")
		updatedLB, err := r.CreateOrUpdateLB(ctx, *lb)
		if err != nil {
			log.Error(err, "failed to update LB")
			return "", 0, err
		}
		if needLB && frontendIP == "" {
			frontendIP, err = findFrontendIP(updatedLB, names.frontendName)
			if err != nil {
				log.Error(err, "failed to find frontend ip")
				return "", 0, err
			} else if frontendIP == "" {
				return "", 0, fmt.Errorf("frontend ip not found even after updating lb")
			}
		}
	}

	return frontendIP, lbPort, nil
}

func findFrontendIP(
	lb *network.LoadBalancer,
	frontendName string,
) (string, error) {
	for _, frontendConfig := range lb.Properties.FrontendIPConfigurations {
		if strings.EqualFold(*frontendConfig.Name, frontendName) {
			if frontendConfig.Properties == nil ||
				frontendConfig.Properties.PrivateIPAddressVersion == nil ||
				*frontendConfig.Properties.PrivateIPAddressVersion != network.IPVersionIPv4 ||
				frontendConfig.Properties.PrivateIPAddress == nil {
				return "", fmt.Errorf("found frontend(%s) with unexpected configuration", frontendName)
			} else {
				return *frontendConfig.Properties.PrivateIPAddress, nil
			}
		}
	}
	return "", nil
}

func getExpectedLBRule(lbRuleName, frontendID, backendID, probeID *string) *network.LoadBalancingRule {
	ruleProp := &network.LoadBalancingRulePropertiesFormat{
		Protocol:         to.Ptr(network.TransportProtocolUDP),
		EnableFloatingIP: to.Ptr(true),
		FrontendIPConfiguration: &network.SubResource{
			ID: frontendID,
		},
		BackendAddressPool: &network.SubResource{
			ID: backendID,
		},
		Probe: &network.SubResource{
			ID: probeID,
		},
	}
	return &network.LoadBalancingRule{
		Name:       lbRuleName,
		Properties: ruleProp,
	}
}

func getExpectedLBProbe(
	probeName *string,
	lbProbePort int,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) *network.Probe {
	gatewayUID := ""
	for _, refer := range lbConfig.OwnerReferences {
		// there should be only one ownerReference actually
		if refer.Name == lbConfig.Name {
			gatewayUID = string(refer.UID)
			break
		}
	}
	probeProp := &network.ProbePropertiesFormat{
		RequestPath: to.Ptr(consts.GatewayHealthProbeEndpoint + gatewayUID),
		Protocol:    to.Ptr(network.ProbeProtocolHTTP),
		Port:        to.Ptr(int32(lbProbePort)),
	}
	return &network.Probe{
		Name:       probeName,
		Properties: probeProp,
	}
}

func getExpectedFrontendConfig(
	frontendName *string,
	subnetID *string,
) *network.FrontendIPConfiguration {
	frontendProp := &network.FrontendIPConfigurationPropertiesFormat{
		PrivateIPAddressVersion:   to.Ptr(network.IPVersionIPv4),
		PrivateIPAllocationMethod: to.Ptr(network.IPAllocationMethodDynamic),
		Subnet: &network.Subnet{
			ID: subnetID,
		},
	}
	return &network.FrontendIPConfiguration{
		Name:       frontendName,
		Properties: frontendProp,
	}
}

func getExpectedBackendPool(
	backendPoolName *string,
) *network.BackendAddressPool {
	return &network.BackendAddressPool{
		Name:       backendPoolName,
		Properties: &network.BackendAddressPoolPropertiesFormat{},
	}
}

func sameLBRuleConfig(ctx context.Context, lbRule1, lbRule2 *network.LoadBalancingRule) bool {
	log := log.FromContext(ctx)
	equalSubResource := func(s, t *network.SubResource) bool {
		if s == nil && t == nil {
			return true
		}
		if s == nil || t == nil {
			return false
		}
		return strings.EqualFold(to.Val(s.ID), to.Val(t.ID))
	}

	if lbRule1.Properties == nil && lbRule2.Properties == nil {
		return true
	}
	if lbRule1.Properties == nil || lbRule2.Properties == nil {
		return false
	}
	if !equalSubResource(lbRule1.Properties.FrontendIPConfiguration, lbRule2.Properties.FrontendIPConfiguration) {
		log.Info("lb rule frontendIPConfigurations are different")
		return false
	}
	if !equalSubResource(lbRule1.Properties.BackendAddressPool, lbRule2.Properties.BackendAddressPool) {
		log.Info("lb rule backendAddressPools are different")
		return false
	}
	if !equalSubResource(lbRule1.Properties.Probe, lbRule2.Properties.Probe) {
		log.Info("lb rule probes are different")
		return false
	}
	if to.Val(lbRule1.Properties.Protocol) != to.Val(lbRule2.Properties.Protocol) {
		log.Info("lb rule protocols are different")
		return false
	}
	if to.Val(lbRule1.Properties.EnableFloatingIP) != to.Val(lbRule2.Properties.EnableFloatingIP) {
		log.Info("lb rule enableFloatingIPs are different")
		return false
	}
	return true
}

func selectPortForLBRule(targetRule *network.LoadBalancingRule, lbRules []*network.LoadBalancingRule) (int32, error) {
	ports := make([]bool, consts.WireguardPortEnd-consts.WireguardPortStart)
	for _, rule := range lbRules {
		if rule.Properties != nil && rule.Properties.BackendAddressPool != nil &&
			strings.EqualFold(to.Val(rule.Properties.BackendAddressPool.ID), to.Val(targetRule.Properties.BackendAddressPool.ID)) {
			if rule.Properties.FrontendPort == nil || rule.Properties.BackendPort == nil || *rule.Properties.FrontendPort != *rule.Properties.BackendPort ||
				*rule.Properties.BackendPort < consts.WireguardPortStart || *rule.Properties.BackendPort >= consts.WireguardPortEnd {
				return 0, fmt.Errorf("selectPortForLBRule: found rule with invalid LB port")
			}
			ports[*rule.Properties.BackendPort-consts.WireguardPortStart] = true
		}
	}
	for i, portInUse := range ports {
		if !portInUse {
			return int32(i) + consts.WireguardPortStart, nil
		}
	}
	return 0, fmt.Errorf("selectPortForLBRule: No available ports")
}

func (r *GatewayLBConfigurationReconciler) reconcileGatewayVMConfig(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) error {
	log := log.FromContext(ctx)

	vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      lbConfig.Name,
			Namespace: lbConfig.Namespace,
		},
	}
	if _, err := controllerutil.CreateOrPatch(ctx, r, vmConfig, func() error {
		vmConfig.Spec.GatewayNodepoolName = lbConfig.Spec.GatewayNodepoolName
		vmConfig.Spec.GatewayVmssProfile = lbConfig.Spec.GatewayVmssProfile
		vmConfig.Spec.ProvisionPublicIps = lbConfig.Spec.ProvisionPublicIps
		vmConfig.Spec.PublicIpPrefixId = lbConfig.Spec.PublicIpPrefixId
		return controllerutil.SetControllerReference(lbConfig, vmConfig, r.Client.Scheme())
	}); err != nil {
		log.Error(err, "failed to reconcile gateway vm configuration")
		return err
	}

	// Collect status from vmConfig to staticGatewayConfiguration
	if vmConfig.DeletionTimestamp.IsZero() && vmConfig.Status != nil {
		if lbConfig.Status == nil {
			lbConfig.Status = &egressgatewayv1alpha1.GatewayLBConfigurationStatus{}
		}
		lbConfig.Status.EgressIpPrefix = vmConfig.Status.EgressIpPrefix
	}

	return nil
}
