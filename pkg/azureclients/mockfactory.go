package azureclients

import (
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/loadbalancerclient/mockloadbalancerclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient/mockpublicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssclient/mockvmssclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient/mockvmssvmclient"
	"github.com/golang/mock/gomock"
)

type MockAzureClientsFactory struct {
	ctrl *gomock.Controller
}

func NewMockAzureClientsFactory(ctrl *gomock.Controller) AzureClientsFactory {
	return &MockAzureClientsFactory{ctrl: ctrl}
}

func (factory *MockAzureClientsFactory) GetLoadBalancersClient() (loadbalancerclient.Interface, error) {
	return mockloadbalancerclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetVirtualMachineScaleSetsClient() (vmssclient.Interface, error) {
	return mockvmssclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetVirtualMachineScaleSetVMsClient() (vmssvmclient.Interface, error) {
	return mockvmssvmclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetPublicIPPrefixesClient() (publicipprefixclient.Interface, error) {
	return mockpublicipprefixclient.NewMockInterface(factory.ctrl), nil
}
