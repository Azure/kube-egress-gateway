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

package azureclients

import (
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/interfaceclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/interfaceclient/mockinterfaceclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/publicipprefixclient/mockpublicipprefixclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/subnetclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/subnetclient/mocksubnetclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient"
	"github.com/Azure/kube-egress-gateway/pkg/azureclients/vmssvmclient/mockvmssvmclient"
	"github.com/golang/mock/gomock"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/loadbalancerclient/mock_loadbalancerclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/virtualmachinescalesetclient/mock_virtualmachinescalesetclient"
)

type MockAzureClientsFactory struct {
	ctrl *gomock.Controller
}

func NewMockAzureClientsFactory(ctrl *gomock.Controller) AzureClientsFactory {
	return &MockAzureClientsFactory{ctrl: ctrl}
}

func (factory *MockAzureClientsFactory) GetLoadBalancersClient() (loadbalancerclient.Interface, error) {
	return mock_loadbalancerclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetVirtualMachineScaleSetsClient() (virtualmachinescalesetclient.Interface, error) {
	return mock_virtualmachinescalesetclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetVirtualMachineScaleSetVMsClient() (vmssvmclient.Interface, error) {
	return mockvmssvmclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetPublicIPPrefixesClient() (publicipprefixclient.Interface, error) {
	return mockpublicipprefixclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetInterfacesClient() (interfaceclient.Interface, error) {
	return mockinterfaceclient.NewMockInterface(factory.ctrl), nil
}

func (factory *MockAzureClientsFactory) GetSubnetsClient() (subnetclient.Interface, error) {
	return mocksubnetclient.NewMockInterface(factory.ctrl), nil
}
