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

package controllers

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

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2021-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2021-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	kubeegressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/azmanager"
)

// GatewayLBConfigurationReconciler reconciles a GatewayLBConfiguration object
type GatewayLBConfigurationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	azmanager.AzureManager
}

type lbPropertyNames struct {
	frontendName string
	backendName  string
	lbRuleName   string
	probeName    string
}

//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=gatewaylbconfigurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=gatewaylbconfigurations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kube-egress-gateway.microsoft.com,resources=gatewaylbconfigurations/finalizers,verbs=update

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
	lbConfig := &kubeegressgatewayv1alpha1.GatewayLBConfiguration{}
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
		For(&kubeegressgatewayv1alpha1.GatewayLBConfiguration{}).
		Complete(r)
}

func (r *GatewayLBConfigurationReconciler) reconcile(
	ctx context.Context,
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling GatewayLBConfiguration %s/%s", lbConfig.Namespace, lbConfig.Name))

	if !controllerutil.ContainsFinalizer(lbConfig, LBConfigFinalizerName) {
		log.Info("Adding finalizer")
		controllerutil.AddFinalizer(lbConfig, LBConfigFinalizerName)
		err := r.Update(ctx, lbConfig)
		if err != nil {
			log.Error(err, "failed to add finalizer")
		}
		return ctrl.Result{}, err
	}

	existing := &kubeegressgatewayv1alpha1.GatewayLBConfiguration{}
	lbConfig.DeepCopyInto(existing)

	// reconcile LB rule
	ip, port, err := r.reconcileLBRule(ctx, lbConfig, true)
	if err != nil {
		log.Error(err, "failed to reconcile LB rules")
		return ctrl.Result{}, err
	}
	lbConfig.Status.FrontendIP = ip
	lbConfig.Status.ServerPort = int(port)

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
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info(fmt.Sprintf("Reconciling gatewayLBConfiguration deletion %s/%s", lbConfig.Namespace, lbConfig.Name))

	if !controllerutil.ContainsFinalizer(lbConfig, LBConfigFinalizerName) {
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
	controllerutil.RemoveFinalizer(lbConfig, LBConfigFinalizerName)
	if err := r.Update(ctx, lbConfig); err != nil {
		log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("gatewayLBConfiguration deletion reconciled")
	return ctrl.Result{}, nil
}

func getLBPropertyName(
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
	vmss *compute.VirtualMachineScaleSet,
) (*lbPropertyNames, error) {
	if vmss.UniqueID == nil {
		return nil, fmt.Errorf("gateway vmss does not have UID")
	}
	names := &lbPropertyNames{
		frontendName: *vmss.UniqueID,
		backendName:  *vmss.UniqueID,
		lbRuleName:   string(lbConfig.GetUID()),
		probeName:    string(lbConfig.GetUID()),
	}
	return names, nil
}

func (r *GatewayLBConfigurationReconciler) getGatewayVMSS(
	ctx context.Context,
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
) (*compute.VirtualMachineScaleSet, error) {
	if lbConfig.Spec.GatewayNodepoolName != "" {
		vmssList, err := r.ListVMSS()
		if err != nil {
			return nil, err
		}
		for i := range vmssList {
			vmss := vmssList[i]
			if v, ok := vmss.Tags[AKSNodepoolTagKey]; ok {
				if strings.EqualFold(to.String(v), lbConfig.Spec.GatewayNodepoolName) {
					return &vmss, nil
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
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
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
	vmss, err := r.getGatewayVMSS(ctx, lbConfig)
	if err != nil {
		log.Error(err, "failed to get vmssID")
		return "", 0, err
	}

	// get lbPropertyNames
	names, err := getLBPropertyName(lbConfig, vmss)
	if err != nil {
		log.Error(err, "failed to get LB property names")
		return "", 0, err
	}

	if lb.LoadBalancerPropertiesFormat == nil {
		return "", 0, fmt.Errorf("lb property is empty")
	}

	frontendID := r.GetLBFrontendIPConfigurationID(names.frontendName)
	if lb.FrontendIPConfigurations != nil {
		for _, frontendConfig := range *lb.FrontendIPConfigurations {
			if strings.EqualFold(*frontendConfig.Name, names.frontendName) &&
				strings.EqualFold(*frontendConfig.ID, *frontendID) &&
				frontendConfig.FrontendIPConfigurationPropertiesFormat != nil &&
				frontendConfig.PrivateIPAddressVersion == network.IPVersionIPv4 &&
				frontendConfig.PrivateIPAddress != nil {
				log.Info("Found LB frontendIPConfiguration", "frontendName", names.frontendName)
				frontendIP = *frontendConfig.PrivateIPAddress
				break
			}
		}
	}
	if frontendIP == "" {
		return "", 0, fmt.Errorf("failed to find corresponding LB frontend IPConfig")
	}

	backendID := r.GetLBBackendAddressPoolID(names.backendName)
	foundBackend := false
	if lb.BackendAddressPools != nil {
		for _, backendPool := range *lb.BackendAddressPools {
			if strings.EqualFold(*backendPool.Name, names.backendName) &&
				strings.EqualFold(*backendPool.ID, *backendID) {
				log.Info("Found LB backendAddressPool", "backendName", names.backendName)
				foundBackend = true
				break
			}
		}
	}
	if !foundBackend {
		return "", 0, fmt.Errorf("failed to find corresponding LB backend address pool")
	}

	probeID := r.GetLBProbeID(names.probeName)
	expectedLBRule := getExpectedLBRule(&names.lbRuleName, frontendID, backendID, probeID)
	expectedProbe := getExpectedLBProbe(&names.probeName, lbConfig)

	lbRules := make([]network.LoadBalancingRule, 0)
	if lb.LoadBalancingRules != nil {
		lbRules = *lb.LoadBalancingRules
	}
	if needLB {
		foundRule := false
		for i := range lbRules {
			lbRule := lbRules[i]
			if strings.EqualFold(*lbRule.Name, *expectedLBRule.Name) {
				if !sameLBRuleConfig(ctx, &lbRule, expectedLBRule) {
					log.Info("Found LB rule with different configuration, dropping")
					lbRules = append(lbRules[:i], lbRules[i+1:]...)
				} else {
					log.Info("Found expected LB rule, keeping")
					foundRule = true
					lbPort = *lbRule.FrontendPort
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
			expectedLBRule.FrontendPort = &port
			expectedLBRule.BackendPort = &port
			lbRules = append(lbRules, *expectedLBRule)
			lb.LoadBalancingRules = &lbRules
			updateLB = true
		}
	} else {
		for i := range lbRules {
			lbRule := lbRules[i]
			if strings.EqualFold(*lbRule.Name, *expectedLBRule.Name) {
				log.Info("Found LB rule, dropping")
				lbRules = append(lbRules[:i], lbRules[i+1:]...)
				updateLB = true
				lb.LoadBalancingRules = &lbRules
				break
			}
		}
	}

	probes := make([]network.Probe, 0)
	if lb.Probes != nil {
		probes = *lb.Probes
	}
	if needLB {
		foundProbe := false
		for i := range probes {
			probe := probes[i]
			if strings.EqualFold(*probe.Name, *expectedProbe.Name) {
				if to.String(probe.RequestPath) != to.String(expectedProbe.RequestPath) ||
					to.Int32(probe.Port) != to.Int32(expectedProbe.Port) ||
					probe.Protocol != expectedProbe.Protocol {
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
			probes = append(probes, *expectedProbe)
			lb.Probes = &probes
			updateLB = true
		}
	} else {
		for i := range probes {
			probe := probes[i]
			if strings.EqualFold(*probe.Name, *expectedProbe.Name) {
				log.Info("Found LB probe, dropping")
				probes = append(probes[:i], probes[i+1:]...)
				updateLB = true
				lb.Probes = &probes
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
		Protocol:         network.TransportProtocolUDP,
		EnableFloatingIP: to.BoolPtr(true),
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
		Name:                              lbRuleName,
		LoadBalancingRulePropertiesFormat: ruleProp,
	}
}

func getExpectedLBProbe(
	probeName *string,
	lbConfig *kubeegressgatewayv1alpha1.GatewayLBConfiguration,
) *network.Probe {
	probeProp := &network.ProbePropertiesFormat{
		RequestPath: to.StringPtr("/" + lbConfig.Namespace + "/" + lbConfig.Name),
		Protocol:    network.ProbeProtocolHTTP,
		Port:        to.Int32Ptr(WireguardDaemonServicePort),
	}
	return &network.Probe{
		Name:                  probeName,
		ProbePropertiesFormat: probeProp,
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
		return strings.EqualFold(to.String(s.ID), to.String(t.ID))
	}

	if !equalSubResource(lbRule1.FrontendIPConfiguration, lbRule2.FrontendIPConfiguration) {
		log.Info("lb rule frontendIPConfigurations are different")
		return false
	}
	if !equalSubResource(lbRule1.BackendAddressPool, lbRule2.BackendAddressPool) {
		log.Info("lb rule backendAddressPools are different")
		return false
	}
	if !equalSubResource(lbRule1.Probe, lbRule2.Probe) {
		log.Info("lb rule probes are different")
		return false
	}
	return true
}

func selectPortForLBRule(targetRule *network.LoadBalancingRule, lbRules []network.LoadBalancingRule) (int32, error) {
	ports := make([]bool, WireguardPortEnd-WireguardPortStart)
	for _, rule := range lbRules {
		if rule.BackendAddressPool != nil &&
			strings.EqualFold(to.String(rule.BackendAddressPool.ID), to.String(targetRule.BackendAddressPool.ID)) {
			if rule.FrontendPort == nil || rule.BackendPort == nil || *rule.FrontendPort != *rule.BackendPort ||
				*rule.BackendPort < WireguardPortStart || *rule.BackendPort >= WireguardPortEnd {
				return 0, fmt.Errorf("selectPortForLBRule: found rule with invalid LB port")
			}
			ports[*rule.BackendPort-WireguardPortStart] = true
		}
	}
	for i, portInUse := range ports {
		if !portInUse {
			return int32(i + WireguardPortStart), nil
		}
	}
	return 0, fmt.Errorf("selectPortForLBRule: No available ports")
}
