// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
)

var _ = Describe("Daemon RBAC", func() {
	It("allows only the manager to write gateway configuration status across namespaces", func() {
		ctx := context.Background()
		namespaces := make([]corev1.Namespace, 2)
		for i := range namespaces {
			namespaces[i].GenerateName = "daemon-rbac-"
			Expect(k8sClient.Create(ctx, &namespaces[i])).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, &namespaces[i])
		}

		newRoleClient := func(component string) client.Client {
			file, err := os.Open(filepath.Join("..", "..", "config", component, "rbac", "role.yaml"))
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(file.Close)

			role := &rbacv1.ClusterRole{}
			Expect(yaml.NewYAMLOrJSONDecoder(file, 4096).Decode(role)).To(Succeed())
			Expect(role.Kind).To(Equal("ClusterRole"))
			role.Name += "-rbac-test"
			Expect(k8sClient.Create(ctx, role)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, role)

			account := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{Name: component, Namespace: namespaces[0].Name},
			}
			Expect(k8sClient.Create(ctx, account)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, account)

			binding := &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: role.Name},
				RoleRef: rbacv1.RoleRef{
					APIGroup: rbacv1.GroupName,
					Kind:     "ClusterRole",
					Name:     role.Name,
				},
				Subjects: []rbacv1.Subject{{
					Kind:      "ServiceAccount",
					Name:      account.Name,
					Namespace: account.Namespace,
				}},
			}
			Expect(k8sClient.Create(ctx, binding)).To(Succeed())
			DeferCleanup(k8sClient.Delete, ctx, binding)

			config := rest.CopyConfig(cfg)
			config.Impersonate.UserName = "system:serviceaccount:" + account.Namespace + ":" + account.Name
			roleClient, err := client.New(config, client.Options{Scheme: scheme.Scheme})
			Expect(err).NotTo(HaveOccurred())
			return roleClient
		}

		daemonClient := newRoleClient("daemon")
		managerClient := newRoleClient("manager")
		gateway := &egressgatewayv1alpha1.StaticGatewayConfiguration{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: namespaces[1].Name},
			Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
				GatewayNodepoolName: "other-pool",
			},
		}
		Expect(k8sClient.Create(ctx, gateway)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, gateway)

		for _, roleClient := range []client.Client{daemonClient, managerClient} {
			Eventually(func() error {
				return roleClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)
			}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
		}

		patch := client.RawPatch(types.MergePatchType, []byte(`{"status":{"egressIpPrefix":"192.0.2.0/31"}}`))
		Expect(apierrors.IsForbidden(daemonClient.Status().Patch(ctx, gateway.DeepCopy(), patch))).To(BeTrue())
		updated := gateway.DeepCopy()
		updated.Status.EgressIpPrefix = "192.0.2.0/31"
		Expect(apierrors.IsForbidden(daemonClient.Status().Update(ctx, updated))).To(BeTrue())

		Expect(managerClient.Status().Patch(ctx, gateway, patch)).To(Succeed())
		gateway.Status.EgressIpPrefix = "192.0.2.2/31"
		Expect(managerClient.Status().Update(ctx, gateway)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gateway), gateway)).To(Succeed())
		Expect(gateway.Status.EgressIpPrefix).To(Equal("192.0.2.2/31"))

		endpoint := &egressgatewayv1alpha1.PodEndpoint{
			ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: namespaces[1].Name},
		}
		Expect(k8sClient.Create(ctx, endpoint)).To(Succeed())
		DeferCleanup(k8sClient.Delete, ctx, endpoint)
		Expect(daemonClient.Status().Patch(ctx, endpoint, client.RawPatch(types.MergePatchType, []byte(`{"status":{}}`)))).To(Succeed())
		Expect(daemonClient.Status().Update(ctx, endpoint)).To(Succeed())
	})
})
