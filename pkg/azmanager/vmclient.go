// ListVMs lists all the virtual machines in the specified resource group.
func (az *AzureManager) ListVMs(ctx context.Context) ([]*compute.VirtualMachine, error) {
	logger := log.FromContext(ctx).WithValues("operation", "ListVMs", "resourceGroup", az.LoadBalancerResourceGroup)
	ctx = log.IntoContext(ctx, logger)
	var vmsList []*compute.VirtualMachine
	err := wrapRetry(ctx, "ListVMs", func(ctx context.Context) error {
		var err error
		vmsList, err = az.VmClient.List(ctx, az.ResourceGroup)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return vmsList, nil
}

// GetVM gets a virtual machine by name.
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
		vm, err = az.VmClient.Get(ctx, resourceGroup, vmName, nil)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return vm, nil
}

// CreateOrUpdateVM creates or updates a virtual machine.
func (az *AzureManager) CreateOrUpdateVM(ctx context.Context, resourceGroup, vmName string, vm compute.VirtualMachine) (*compute.VirtualMachine, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmName == "" {
		return nil, fmt.Errorf("vm name is empty")
	}
	
	logger := log.FromContext(ctx).WithValues("operation", "CreateOrUpdateVM", "resourceGroup", resourceGroup, "resourceName", vmName)
	ctx = log.IntoContext(ctx, logger)
	var retVm *compute.VirtualMachine
	err := wrapRetry(ctx, "CreateOrUpdateVM", func(ctx context.Context) error {
		var err error
		retVm, err = az.VmClient.CreateOrUpdate(ctx, resourceGroup, vmName, vm)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return retVm, nil
}
//
// GetVMInterface gets a network interface for a VM.
func (az *AzureManager) GetVMInterface(ctx context.Context, resourceGroup, vmName, nicName string) (*network.Interface, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmName == "" {
		return nil, fmt.Errorf("vm name is empty")
	}
	if nicName == "" {
		return nil, fmt.Errorf("nic name is empty")
	}
	logger := log.FromContext(ctx).WithValues("operation", "GetVMInterface", "resourceGroup", resourceGroup, "vmName", vmName, "nicName", nicName)
	ctx = log.IntoContext(ctx, logger)

	var nic *network.Interface
	err := wrapRetry(ctx, "GetVMInterface", func(ctx context.Context) error {
		var err error
		nic, err = az.InterfaceClient.Get(ctx, resourceGroup, nicName)
		return err
	}, isRateLimitError)
	if err != nil {
		return nil, err
	}
	return nic, nil
}

// UpdateVM updates a virtual machine.
func (az *AzureManager) UpdateVM(ctx context.Context, resourceGroup, vmName string, vmUpdate compute.VirtualMachineUpdate) (*compute.VirtualMachine, error) {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmName == "" {
		return nil, fmt.Errorf("vm name is empty")
	}

	logger := log.FromContext(ctx).WithValues("operation", "UpdateVM", "resourceGroup", resourceGroup, "resourceName", vmName)
	ctx = log.IntoContext(ctx, logger)

	var retVm *compute.VirtualMachine
	err := wrapRetry(ctx, "UpdateVM", func(ctx context.Context) error {
		var err error
		retVm, err = az.VmClient.Update(ctx, resourceGroup, vmName, vmUpdate)
		return err
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	if err != nil {
		return nil, err
	}
	return retVm, nil
}

// DeleteVM deletes a virtual machine.
func (az *AzureManager) DeleteVM(ctx context.Context, resourceGroup, vmName string) error {
	if resourceGroup == "" {
		resourceGroup = az.ResourceGroup
	}
	if vmName == "" {
		return fmt.Errorf("vm name is empty")
	}

	logger := log.FromContext(ctx).WithValues("operation", "DeleteVM", "resourceGroup", resourceGroup, "resourceName", vmName)
	ctx = log.IntoContext(ctx, logger)

	err := wrapRetry(ctx, "DeleteVM", func(ctx context.Context) error {
		return az.VmClient.Delete(ctx, resourceGroup, vmName)
	}, isRateLimitError, retrySettings{OverallTimeout: to.Ptr(5 * time.Minute)})
	
	return err
}
