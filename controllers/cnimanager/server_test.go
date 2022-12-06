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
package cnimanager_test

import (
	"context"

	current "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/cnimanager"
	cniprotocol "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Server", func() {
	var service *cnimanager.NicService
	var fakeClient client.Client
	var nicAddInputRequest *cniprotocol.NicAddRequest
	var nicDelInputRequest *cniprotocol.NicDelRequest
	var gatewayProfile *current.StaticGatewayConfiguration
	BeforeEach(func() {
		fakeClientBuilder := fake.NewClientBuilder()
		apischeme := runtime.NewScheme()
		utilruntime.Must(current.AddToScheme(apischeme))
		fakeClientBuilder.WithScheme(apischeme)
		gatewayProfile = &current.StaticGatewayConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tgw1",
				Namespace: "default",
			},
			Status: current.StaticGatewayConfigurationStatus{
				GatewayWireguardProfile: current.GatewayWireguardProfile{
					WireguardServerIP:   "192.168.1.1/32",
					WireguardPublicKey:  "somerandompublickey",
					WireguardServerPort: 54321,
				},
			},
		}
		nicAddInputRequest = &cniprotocol.NicAddRequest{
			PodConfig: &cniprotocol.PodInfo{
				PodName:      "test",
				PodNamespace: "default",
			},
			ListenPort:  12345,
			AllowedIp:   "192.168.1.10/32",
			PublicKey:   "SOMERANDOMPUBLICKKEY",
			GatewayName: gatewayProfile.Name,
		}
		nicDelInputRequest = &cniprotocol.NicDelRequest{
			PodConfig: nicAddInputRequest.PodConfig,
		}
		fakeClientBuilder.WithRuntimeObjects(gatewayProfile)
		fakeClient = fakeClientBuilder.Build()
		service = cnimanager.NewNicService(fakeClient)
	})
	Context("when nic is created", func() {
		When("gateway is found", func() {
			It("should fetch gateway and create pod endpoint", func() {
				resp, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.PublicKey).To(Equal(gatewayProfile.Status.GatewayWireguardProfile.WireguardPublicKey))
				Expect(resp.EndpointIp).To(Equal(gatewayProfile.Status.GatewayWireguardProfile.WireguardServerIP))
				Expect(resp.ListenPort).To(Equal(gatewayProfile.Status.GatewayWireguardProfile.WireguardServerPort))
				podEndpoint := &current.PodWireguardEndpoint{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      nicAddInputRequest.PodConfig.PodName,
					Namespace: nicAddInputRequest.PodConfig.PodNamespace,
				}, podEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(podEndpoint.Spec.StaticGatewayConfiguration).To(Equal(gatewayProfile.Name))
				Expect(podEndpoint.Spec.PodWireguardPublicKey).To(Equal(nicAddInputRequest.PublicKey))
				Expect(podEndpoint.Spec.PodIpAddress).To(Equal(nicAddInputRequest.AllowedIp))
			})
		})
		When("gateway is not found", func() {
			It("should return error and don't create pod endpoint", func() {
				fakeClient.Delete(context.Background(), gatewayProfile) //nolint:errcheck
				_, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).To(HaveOccurred())
				podEndpoint := &current.PodWireguardEndpoint{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      nicAddInputRequest.PodConfig.PodName,
					Namespace: nicAddInputRequest.PodConfig.PodNamespace,
				}, podEndpoint)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("when nic is deleted", func() {
		When("pod endpoint is not found", func() {
			It("should return nothing", func() {
				_, err := service.NicDel(context.Background(), nicDelInputRequest)
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})
})
