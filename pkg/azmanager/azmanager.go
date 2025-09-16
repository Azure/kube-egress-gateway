// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package azmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	compute "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v6"
	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v6"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/interfaceclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/subnetclient"
	_ "sigs.k8s.io/cloud-provider-azure/pkg/azclient/trace"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachineclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetvmclient"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/kube-egress-gateway/pkg/config"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

const (
	// LB frontendIPConfiguration ID template
	LBFrontendIPConfigTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/frontendIPConfigurations/%s"
	// LB backendAddressPool ID template
	LBBackendPoolIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/backendAddressPools/%s"
	// LB probe ID template
	LBProbeIDTemplate = "/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Network/loadBalancers/%s/probes/%s"

	defaultPollOverallTimeout = 2 * time.Minute

	ErrRateLimitReached = "rate limit reached"
)

var (
	defaultPollBackoff = wait.Backoff{
		Duration: 1 * time.Second,  // initial retry interval
		Factor:   2.0,              // backoff factor
		Jitter:   0.1,              // random jitter
		Steps:    10,               // max retries
		Cap:      30 * time.Second, // max retry internal, limiting exponential growth
	}
)

func isRateLimitError(err error) bool {
	return err.Error() == ErrRateLimitReached
}

func isInternalServerError(err error) bool {
	var respErr *azcore.ResponseError
	return errors.As(err, &respErr) && respErr.ErrorCode == "InternalServerError"
}

type retrySettings struct {
	OverallTimeout *time.Duration
	Backoff        *wait.Backoff
}

func wrapRetry(ctx context.Context, operationName string, operation func(context.Context) error, isRetriableFunc func(error) bool, retrySettings ...retrySettings) error {
	overallTimeout := defaultPollOverallTimeout
	backoff := defaultPollBackoff
	if len(retrySettings) > 0 {
		if retrySettings[0].OverallTimeout != nil {
			overallTimeout = *retrySettings[0].OverallTimeout
		}
		if retrySettings[0].Backoff != nil {
			backoff = *retrySettings[0].Backoff
		}
	}

	conditionFunc := func(ctx context.Context) (bool, error) {
		var err error
		logger := log.FromContext(ctx)
		err = operation(ctx)
		if err != nil {
			if isRetriableFunc(err) {
				logger.Info(fmt.Sprintf("%s retriable error", operationName), "error", err.Error(), "level", "warning")
				return false, nil
			}
			logger.Info(fmt.Sprintf("%s nonretriable error", operationName), "error", err.Error(), "level", "warning")
			return false, err
		}
		logger.Info(fmt.Sprintf("%s success", operationName))
		return true, nil
	}

	ctx, cancel := context.WithTimeout(ctx, overallTimeout)
	defer cancel()
	return wait.ExponentialBackoffWithContext(ctx, backoff, conditionFunc)
}

type AzureManager struct {
	*config.CloudConfig

	LoadBalancerClient   loadbalancerclient.Interface
	VmssClient           virtualmachinescalesetclient.Interface
	VmssVMClient         virtualmachinescalesetvmclient.Interface
	VMClient             virtualmachineclient.Interface
	PublicIPPrefixClient publicipprefixclient.Interface
	InterfaceClient      interfaceclient.Interface
	SubnetClient         subnetclient.Interface
}

func CreateAzureManager(cloud *config.CloudConfig, factory azclient.ClientFactory) (*AzureManager, error) {
	az := AzureManager{
		CloudConfig: cloud,
	}

	az.LoadBalancerClient = factory.GetLoadBalancerClient()
	az.VmssClient = factory.GetVirtualMachineScaleSetClient()
	az.PublicIPPrefixClient = factory.GetPublicIPPrefixClient()
	az.VmssVMClient = factory.GetVirtualMachineScaleSetVMClient()
	az.InterfaceClient = factory.GetInterfaceClient()
	az.SubnetClient = factory.GetSubnetClient()
	az.VMClient = factory.GetVirtualMachineClient()

	return &az, nil
}

func (az *AzureManager) SubscriptionID() string {
	return az.CloudConfig.SubscriptionID
}

func (az *AzureManager) Location() string {
	return az.CloudConfig.Location
}

func (az *AzureManager) LoadBalancerName() string {
	if az.CloudConfig.LoadBalancerName == "" {
		return consts.DefaultGatewayLBName
	}
	return az.CloudConfig.LoadBalancerName
}

func (az *AzureManager) GetLBFrontendIPConfigurationID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBFrontendIPConfigTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLBBackendAddressPoolID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBBackendPoolIDTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLBProbeID(name string) *string {
	return to.Ptr(fmt.Sprintf(LBProbeIDTemplate, az.SubscriptionID(), az.LoadBalancerResourceGroup, az.LoadBalancerName(), name))
}

func (az *AzureManager) GetLB(ctx context.Context) (*network.LoadBalancer, error) {
	logger := log.FromContext(ctx).WithValues("operation", "GetLB", "resourceGroup", az.LoadBalancerResourceGroup, "resourceName", az.LoadBalancerName())
	ctx = log.IntoContext(ctx, logger)

	var ret *network.LoadBalancer
	err := wrapRetry(ctx, "GetLB", func(ctx context.Context) error {
		var err error
		ret, err = az.LoadBalancerClient.Get(ctx, az.LoadBalancerResourceGroup, az.LoadBalancerName(), nil)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (az *AzureManager) CreateOrUpdateLB(ctx context.Context, lb network.LoadBalancer) (*network.LoadBalancer, error) {
	logger := log.FromContext(ctx).WithValues("operation", "CreateOrUpdateLB", "resourceGroup", az.LoadBalancerResourceGroup, "resourceName", to.Val(lb.Name))
	ctx = log.IntoContext(ctx, logger)

	var ret *network.LoadBalancer
	err := wrapRetry(ctx, "CreateOrUpdateLB", func(ctx context.Context) error {
		var err error
		ret, err = az.LoadBalancerClient.CreateOrUpdate(ctx, az.LoadBalancerResourceGroup, to.Val(lb.Name), lb)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func (az *AzureManager) DeleteLB(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("operation", "DeleteLB", "resourceGroup", az.LoadBalancerResourceGroup, "resourceName", az.LoadBalancerName())
	ctx = log.IntoContext(ctx, logger)
	return wrapRetry(ctx, "DeleteLB", func(ctx context.Context) error {
		return az.LoadBalancerClient.Delete(ctx, az.LoadBalancerResourceGroup, az.LoadBalancerName())
	}, isRateLimitError)
}

func (az *AzureManager) ListVMSS(ctx context.Context) ([]*compute.VirtualMachineScaleSet, error) {
	logger := log.FromContext(ctx).WithValues("operation", "ListVMSS", "resourceGroup", az.LoadBalancerResourceGroup)
	ctx = log.IntoContext(ctx, logger)
	var vmssList []*compute.VirtualMachineScaleSet
	err := wrapRetry(ctx, "ListVMSS", func(ctx context.Context) error {
		var err error
		vmssList, err = az.VmssClient.List(ctx, az.ResourceGroup)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return vmssList, nil
}

func (az *AzureManager) GetVMSS(ctx context.Context, resourceGroup, vmssName string) (*compute.VirtualMachineScaleSet, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}

	logger := log.FromContext(ctx).WithValues("operation", "GetVMSS", "resourceGroup", resourceGroup, "resourceName", vmssName)
	ctx = log.IntoContext(ctx, logger)

	var vmss *compute.VirtualMachineScaleSet
	err := wrapRetry(ctx, "GetVMSS", func(ctx context.Context) error {
		var err error
		vmss, err = az.VmssClient.Get(ctx, resourceGroup, vmssName, nil)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return vmss, nil
}

func (az *AzureManager) ListVMs(ctx context.Context) ([]*compute.VirtualMachine, error) {
	logger := log.FromContext(ctx).WithValues("operation", "ListVMs", "resourceGroup", az.LoadBalancerResourceGroup)
	ctx = log.IntoContext(ctx, logger)
	var vmsList []*compute.VirtualMachine
	err := wrapRetry(ctx, "ListVMs", func(ctx context.Context) error {
		var err error
		vmsList, err = az.VMClient.List(ctx, az.ResourceGroup)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return vmsList, nil
}

func (az *AzureManager) GetVM(ctx context.Context, resourceGroup, vmName string) (*compute.VirtualMachine, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmName == "" {
		return nil, fmt.Errorf("vm name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetVM", "resourceGroup", resourceGroup, "resourceName", vmName)
	ctx = log.IntoContext(ctx, logger)

	var vm *compute.VirtualMachine
	err := wrapRetry(ctx, "GetVM", func(ctx context.Context) error {
		var err error
		vm, err = az.VMClient.Get(ctx, resourceGroup, vmName, nil)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return vm, nil
}

func (az *AzureManager) CreateOrUpdateVMSS(ctx context.Context, resourceGroup, vmssName string, vmss compute.VirtualMachineScaleSet) (*compute.VirtualMachineScaleSet, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}

	logger := log.FromContext(ctx).WithValues("operation", "CreateOrUpdateVMSS", "resourceGroup", resourceGroup, "resourceName", vmssName)
	ctx = log.IntoContext(ctx, logger)
	var retVmss *compute.VirtualMachineScaleSet
	err := wrapRetry(ctx, "CreateOrUpdateVMSS", func(ctx context.Context) error {
		var err error
		retVmss, err = az.VmssClient.CreateOrUpdate(ctx, resourceGroup, vmssName, vmss)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return retVmss, nil
}

func (az *AzureManager) ListVMSSInstances(ctx context.Context, resourceGroup, vmssName string) ([]*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "ListVMSSInstances", "resourceGroup", resourceGroup, "resourceName", vmssName)
	ctx = log.IntoContext(ctx, logger)
	var vms []*compute.VirtualMachineScaleSetVM
	err := wrapRetry(ctx, "ListVMSSInstances", func(ctx context.Context) error {
		var err error
		vms, err = az.VmssVMClient.List(ctx, resourceGroup, vmssName)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return vms, nil
}

func (az *AzureManager) GetVMSSInstance(ctx context.Context, resourceGroup, vmssName, instanceID string) (*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("vmss instanceID is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetVMSSInstance", "resourceGroup", resourceGroup, "resourceName", vmssName, "vmssInstanceID", instanceID)
	ctx = log.IntoContext(ctx, logger)
	var vm *compute.VirtualMachineScaleSetVM
	err := wrapRetry(ctx, "GetVMSSInstance", func(ctx context.Context) error {
		var err error
		vm, err = az.VmssVMClient.Get(ctx, resourceGroup, vmssName, instanceID)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return vm, nil
}

func (az *AzureManager) UpdateVMSSInstance(ctx context.Context, resourceGroup, vmssName, instanceID string, vm compute.VirtualMachineScaleSetVM) (*compute.VirtualMachineScaleSetVM, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("vmss instanceID is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "UpdateVMSSInstance", "resourceGroup", resourceGroup, "resourceName", vmssName, "vmssInstanceID", instanceID)
	ctx = log.IntoContext(ctx, logger)
	var retVM *compute.VirtualMachineScaleSetVM
	err := wrapRetry(ctx, "UpdateVMSSInstance", func(ctx context.Context) error {
		var err error
		retVM, err = az.VmssVMClient.Update(ctx, resourceGroup, vmssName, instanceID, vm)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return retVM, nil
}

func (az *AzureManager) GetPublicIPPrefix(ctx context.Context, resourceGroup, prefixName string) (*network.PublicIPPrefix, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return nil, fmt.Errorf("public ip prefix name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetPublicIPPrefix", "resourceGroup", resourceGroup, "resourceName", prefixName)
	ctx = log.IntoContext(ctx, logger)
	var prefix *network.PublicIPPrefix
	err := wrapRetry(ctx, "GetPublicIPPrefix", func(ctx context.Context) error {
		var err error
		prefix, err = az.PublicIPPrefixClient.Get(ctx, resourceGroup, prefixName, nil)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return prefix, nil
}

func (az *AzureManager) CreateOrUpdatePublicIPPrefix(ctx context.Context, resourceGroup, prefixName string, ipPrefix network.PublicIPPrefix) (*network.PublicIPPrefix, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return nil, fmt.Errorf("public ip prefix name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "CreateOrUpdatePublicIPPrefix", "resourceGroup", resourceGroup, "resourceName", prefixName)
	ctx = log.IntoContext(ctx, logger)
	var prefix *network.PublicIPPrefix
	err := wrapRetry(ctx, "CreateOrUpdatePublicIPPrefix", func(ctx context.Context) error {
		var err error
		prefix, err = az.PublicIPPrefixClient.CreateOrUpdate(ctx, resourceGroup, prefixName, ipPrefix)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return prefix, nil
}

func (az *AzureManager) DeletePublicIPPrefix(ctx context.Context, resourceGroup, prefixName string) error {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if prefixName == "" {
		return fmt.Errorf("public ip prefix name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "DeletePublicIPPrefix", "resourceGroup", resourceGroup, "resourceName", prefixName)
	ctx = log.IntoContext(ctx, logger)
	err := wrapRetry(ctx, "DeletePublicIPPrefix", func(ctx context.Context) error {
		return az.PublicIPPrefixClient.Delete(ctx, resourceGroup, prefixName)
	}, func(err error) bool {
		return isRateLimitError(err) || isInternalServerError(err)
	}, retrySettings{OverallTimeout: to.Ptr(15 * time.Minute)})
	return err
}

func (az *AzureManager) GetVMSSInterface(ctx context.Context, resourceGroup, vmssName, instanceID, interfaceName string) (*network.Interface, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmssName == "" {
		return nil, fmt.Errorf("vmss name is empty")
	}
	if instanceID == "" {
		return nil, fmt.Errorf("instanceID is empty")
	}
	if interfaceName == "" {
		return nil, fmt.Errorf("interface name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetVMSSInterface", "resourceGroup", resourceGroup, "resourceName", vmssName, "vmssInstanceID", instanceID, "interfaceName", interfaceName)
	ctx = log.IntoContext(ctx, logger)
	var nicResp *network.Interface
	err := wrapRetry(ctx, "GetVMSSInterface", func(ctx context.Context) error {
		var err error
		nicResp, err = az.InterfaceClient.GetVirtualMachineScaleSetNetworkInterface(ctx, resourceGroup, vmssName, instanceID, interfaceName)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return nicResp, nil
}

func (az *AzureManager) ListNetworkInterfaces(ctx context.Context, resourceGroup string) ([]*network.Interface, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	logger := log.FromContext(ctx).WithValues("operation", "ListNetworkInterfaces", "resourceGroup", resourceGroup)
	ctx = log.IntoContext(ctx, logger)
	var nics []*network.Interface
	err := wrapRetry(ctx, "ListNetworkInterfaces", func(ctx context.Context) error {
		var err error
		nics, err = az.InterfaceClient.List(ctx, resourceGroup)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return nics, nil
}

func (az *AzureManager) GetNetworkInterface(ctx context.Context, interfaceName string) (*network.Interface, error) {
	resourceGroup := az.ResourceGroup
	if interfaceName == "" {
		return nil, fmt.Errorf("interface name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetVMSSInterface", "resourceGroup", resourceGroup, "interfaceName", interfaceName)
	ctx = log.IntoContext(ctx, logger)
	var nicResp *network.Interface
	err := wrapRetry(ctx, "GetNetworkInterface", func(ctx context.Context) error {
		var err error
		nicResp, err = az.InterfaceClient.Get(ctx, resourceGroup, interfaceName, nil)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return nicResp, nil
}

func (az *AzureManager) CreateOrUpdateNetworkInterface(ctx context.Context, resourceGroup, nicName string, networkInterface network.Interface) (*network.Interface, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}

	if nicName == "" {
		return nil, fmt.Errorf("nic name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "CreateOrUpdateNetworkInterface", "resourceGroup", resourceGroup, "resourceName", nicName)
	ctx = log.IntoContext(ctx, logger)
	var nic *network.Interface
	err := wrapRetry(ctx, "CreateOrUpdateNetworkInterface", func(ctx context.Context) error {
		var err error
		nic, err = az.InterfaceClient.CreateOrUpdate(ctx, resourceGroup, nicName, networkInterface)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return nic, nil
}

func (az *AzureManager) GetSubnet(ctx context.Context) (*network.Subnet, error) {
	logger := log.FromContext(ctx).WithValues("operation", "GetSubnet", "resourceGroup", az.VnetResourceGroup, "resourceName", az.VnetName, "subnetName", az.SubnetName)
	ctx = log.IntoContext(ctx, logger)
	var subnet *network.Subnet
	err := wrapRetry(ctx, "GetSubnet", func(ctx context.Context) error {
		var err error
		subnet, err = az.SubnetClient.Get(ctx, az.VnetResourceGroup, az.VnetName, az.SubnetName, nil)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return subnet, nil
}
