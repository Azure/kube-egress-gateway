// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cnimanager_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	current "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/cnimanager"
	cniprotocol "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
)

var _ = Describe("Server", func() {
	var service *cnimanager.NicService
	var fakeClient client.Client
	var nicAddInputRequest *cniprotocol.NicAddRequest
	var nicDelInputRequest *cniprotocol.NicDelRequest
	var podRetrieveRequest *cniprotocol.PodRetrieveRequest
	var gatewayProfile *current.StaticGatewayConfiguration
	var pod *corev1.Pod
	BeforeEach(func() {
		fakeClientBuilder := fake.NewClientBuilder()
		apischeme := runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(apischeme))
		utilruntime.Must(current.AddToScheme(apischeme))
		fakeClientBuilder.WithScheme(apischeme)
		gatewayProfile = &current.StaticGatewayConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "tgw1",
				Namespace: "default",
			},
			Status: current.StaticGatewayConfigurationStatus{
				EgressIpPrefix: "13.66.156.240/30",
				GatewayServerProfile: current.GatewayServerProfile{
					Ip:        "192.168.1.1/32",
					PublicKey: "somerandompublickey",
					Port:      54321,
				},
			},
		}
		pod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
				Annotations: map[string]string{
					"key1": "value1",
					"key2": "value2",
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
		podRetrieveRequest = &cniprotocol.PodRetrieveRequest{
			PodConfig: nicAddInputRequest.PodConfig,
		}
		fakeClientBuilder.WithRuntimeObjects(gatewayProfile, pod)
		fakeClient = fakeClientBuilder.Build()
		service = cnimanager.NewNicService(fakeClient)
	})

	Context("when gateway is not ready", func() {
		BeforeEach(func() {
			gatewayProfile = &current.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "tgw1",
					Namespace: "default",
				},
				Status: current.StaticGatewayConfigurationStatus{
					GatewayServerProfile: current.GatewayServerProfile{
						PublicKey: "somerandompublickey",
					},
				},
			}
			fakeClientBuilder := fake.NewClientBuilder()
			apischeme := runtime.NewScheme()
			utilruntime.Must(current.AddToScheme(apischeme))
			fakeClientBuilder.WithScheme(apischeme)
			fakeClientBuilder.WithRuntimeObjects(gatewayProfile)
			fakeClient = fakeClientBuilder.Build()
			service = cnimanager.NewNicService(fakeClient)
		})
		When("when gateway is not ready", func() {
			It("should return error", func() {
				_, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("when nic is created", func() {
		When("gateway is found", func() {
			It("should fetch gateway and create pod endpoint", func() {
				resp, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.PublicKey).To(Equal(gatewayProfile.Status.GatewayServerProfile.PublicKey))
				Expect(resp.EndpointIp).To(Equal(gatewayProfile.Status.GatewayServerProfile.Ip))
				Expect(resp.ListenPort).To(Equal(gatewayProfile.Status.GatewayServerProfile.Port))
				Expect(resp.DefaultRoute).To(Equal(cniprotocol.DefaultRoute_DEFAULT_ROUTE_STATIC_EGRESS_GATEWAY))
				podEndpoint := &current.PodEndpoint{}
				err = fakeClient.Get(context.Background(), client.ObjectKey{
					Name:      nicAddInputRequest.PodConfig.PodName,
					Namespace: nicAddInputRequest.PodConfig.PodNamespace,
				}, podEndpoint)
				Expect(err).NotTo(HaveOccurred())
				Expect(podEndpoint.Spec.StaticGatewayConfiguration).To(Equal(gatewayProfile.Name))
				Expect(podEndpoint.Spec.PodPublicKey).To(Equal(nicAddInputRequest.PublicKey))
				Expect(podEndpoint.Spec.PodIpAddress).To(Equal(nicAddInputRequest.AllowedIp))
			})
		})
		When("gateway has azureNetworking as default route", func() {
			It("should return default route as azureNetworking", func() {
				gatewayProfile.Spec.DefaultRoute = current.RouteAzureNetworking
				fakeClient.Update(context.Background(), gatewayProfile) //nolint:errcheck
				resp, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.DefaultRoute).To(Equal(cniprotocol.DefaultRoute_DEFAULT_ROUTE_AZURE_NETWORKING))
			})
		})
		When("gateway is not found", func() {
			It("should return error and don't create pod endpoint", func() {
				fakeClient.Delete(context.Background(), gatewayProfile) //nolint:errcheck
				_, err := service.NicAdd(context.Background(), nicAddInputRequest)
				Expect(err).To(HaveOccurred())
				podEndpoint := &current.PodEndpoint{}
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

	Context("requesting pod metadata", func() {
		When("pod is found", func() {
			It("should return pods'annotations", func() {
				resp, err := service.PodRetrieve(context.Background(), podRetrieveRequest)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.GetAnnotations()).To(Equal(pod.Annotations))
			})
		})

		When("pod is not found", func() {
			It("should return error", func() {
				fakeClient.Delete(context.Background(), pod) //nolint:errcheck
				_, err := service.PodRetrieve(context.Background(), podRetrieveRequest)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
