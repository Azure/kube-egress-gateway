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

package manager

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v2"
	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/consts"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

// GatewayLBConfigurationReconciler reconciles a GatewayLBConfiguration object
type GatewayLBConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	*azmanager.AzureManager
}

type lbPropertyNames struct {
	frontendName string
	backendName  string
	lbRuleName   string
	probeName    string
}

//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=egressgateway.kubernetes.azure.com,resources=gatewaylbconfigurations/finalizers,verbs=update

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

	if !lbConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Clean up gatewayLBConfiguration
		return r.ensureDeleted(ctx, lbConfig)
	}

	return r.reconcile(ctx, lbConfig)
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayLBConfigurationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&egressgatewayv1alpha1.GatewayLBConfiguration{}).
		Complete(r)
}

func (r *GatewayLBConfigurationReconciler) reconcile(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling GatewayLBConfiguration %s/%s", lbConfig.Namespace, lbConfig.Name))

	if !controllerutil.ContainsFinalizer(lbConfig, consts.LBConfigFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(lbConfig, consts.LBConfigFinalizerName)
		err := r.Update(ctx, lbConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
		}
		return ctrl.Result{}, err
	}

	existing := &egressgatewayv1alpha1.GatewayLBConfiguration{}
	lbConfig.DeepCopyInto(existing)

	// reconcile LB rule
	ip, port, err := r.reconcileLBRule(ctx, lbConfig, true)
	if err != nil {
		log.Error(err, "failed to reconcile LB rules")
		return ctrl.Result{}, err
	}
	if lbConfig.Status == nil {
		lbConfig.Status = &egressgatewayv1alpha1.GatewayLBConfigurationStatus{}
	}
	lbConfig.Status.FrontendIP = ip
	lbConfig.Status.ServerPort = port

	if !equality.Semantic.DeepEqual(existing, lbConfig) {
		log.Info(fmt.Sprintf("Updating GatewayLBConfiguration %s/%s", lbConfig.Namespace, lbConfig.Name))
		if err := r.Status().Update(ctx, lbConfig); err != nil {
			log.Error(err, "failed to update gateway LB configuration")
		}
	}

	log.Info("GatewayLBConfiguration reconciled")
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
	return ctrl.Result{}, nil
}

func getLBPropertyName(
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
	vmss *compute.VirtualMachineScaleSet,
) (*lbPropertyNames, error) {
	if vmss.Properties == nil || vmss.Properties.UniqueID == nil {
		return nil, fmt.Errorf("gateway vmss does not have UID")
	}
	names := &lbPropertyNames{
		frontendName: *vmss.Properties.UniqueID,
		backendName:  *vmss.Properties.UniqueID,
		lbRuleName:   string(lbConfig.GetUID()),
		probeName:    string(lbConfig.GetUID()),
	}
	return names, nil
}

func (r *GatewayLBConfigurationReconciler) getGatewayVMSS(
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) (*compute.VirtualMachineScaleSet, error) {
	if lbConfig.Spec.GatewayNodepoolName != "" {
		vmssList, err := r.ListVMSS()
		if err != nil {
			return nil, err
		}
		for i := range vmssList {
			vmss := vmssList[i]
			if v, ok := vmss.Tags[consts.AKSNodepoolTagKey]; ok {
				if strings.EqualFold(to.Val(v), lbConfig.Spec.GatewayNodepoolName) {
					return vmss, nil
				}
			}
		}
	} else {
		vmss, err := r.GetVMSS(lbConfig.Spec.VMSSResourceGroup, lbConfig.Spec.VMSSName)
		if err != nil {
			return nil, err
		}
		return vmss, nil
	}
	return nil, fmt.Errorf("gateway VMSS not found")
}

func (r *GatewayLBConfigurationReconciler) reconcileLBRule(
	ctx context.Context,
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
	needLB bool,
) (string, int32, error) {
	// assuming gateway VMSS's corresponding frontend and backend setup are done with VMSS provisioning,
	// just need to reconcile LB rule
	log := log.FromContext(ctx)
	frontendIP := ""
	var lbPort int32
	updateLB := false

	// get LoadBalancer
	lb, err := r.GetLB()
	if err != nil {
		log.Error(err, "failed to get LoadBalancer")
		return "", 0, err
	}

	// get gateway VMSS
	vmss, err := r.getGatewayVMSS(lbConfig)
	if err != nil {
		log.Error(err, "failed to get vmss")
		return "", 0, err
	}

	// get lbPropertyNames
	names, err := getLBPropertyName(lbConfig, vmss)
	if err != nil {
		log.Error(err, "failed to get LB property names")
		return "", 0, err
	}

	if lb.Properties == nil {
		return "", 0, fmt.Errorf("lb property is empty")
	}

	frontendID := r.GetLBFrontendIPConfigurationID(names.frontendName)
	for _, frontendConfig := range lb.Properties.FrontendIPConfigurations {
		if strings.EqualFold(*frontendConfig.Name, names.frontendName) &&
			strings.EqualFold(*frontendConfig.ID, *frontendID) &&
			frontendConfig.Properties != nil &&
			frontendConfig.Properties.PrivateIPAddressVersion != nil &&
			*frontendConfig.Properties.PrivateIPAddressVersion == network.IPVersionIPv4 &&
			frontendConfig.Properties.PrivateIPAddress != nil {
			log.Info("Found LB frontendIPConfiguration", "frontendName", names.frontendName)
			frontendIP = *frontendConfig.Properties.PrivateIPAddress
			break
		}
	}
	if frontendIP == "" {
		return "", 0, fmt.Errorf("failed to find corresponding LB frontend IPConfig")
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
		return "", 0, fmt.Errorf("failed to find corresponding LB backend address pool")
	}

	probeID := r.GetLBProbeID(names.probeName)
	expectedLBRule := getExpectedLBRule(&names.lbRuleName, frontendID, backendID, probeID)
	expectedProbe := getExpectedLBProbe(&names.probeName, lbConfig)

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
		for i := range lbRules {
			lbRule := lbRules[i]
			if strings.EqualFold(*lbRule.Name, *expectedLBRule.Name) {
				log.Info("Found LB rule, dropping")
				lbRules = append(lbRules[:i], lbRules[i+1:]...)
				updateLB = true
				lb.Properties.LoadBalancingRules = lbRules
				break
			}
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

	if updateLB {
		log.Info("Updating load balancer")
		if err := r.CreateOrUpdateLB(*lb); err != nil {
			log.Error(err, "failed to update LB")
			return "", 0, err
		}
	}
	return frontendIP, lbPort, nil
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
	lbConfig *egressgatewayv1alpha1.GatewayLBConfiguration,
) *network.Probe {
	probeProp := &network.ProbePropertiesFormat{
		RequestPath: to.Ptr("/" + lbConfig.Namespace + "/" + lbConfig.Name),
		Protocol:    to.Ptr(network.ProbeProtocolHTTP),
		Port:        to.Ptr(consts.WireguardDaemonServicePort),
	}
	return &network.Probe{
		Name:       probeName,
		Properties: probeProp,
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
