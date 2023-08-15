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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
)

const (
	testName      = "test"
	testNamespace = "testns"
	privK         = "GHuMwljFfqd2a7cs6BaUOmHflK23zME8VNvC5B37S3k="
	pubK          = "aPxGwq8zERHQ3Q1cOZFdJ+cvJX5Ka4mLN38AyYKYF10="
)

var _ = Describe("StaticGatewayConfiguration controller in testenv", Ordered, func() {
	var (
		ctx       context.Context
		cancel    context.CancelFunc
		timeout   = time.Second * 10
		interval  = time.Millisecond * 250
		gwConfig  *egressgatewayv1alpha1.StaticGatewayConfiguration
		namespace *corev1.Namespace
		recorder  = record.NewFakeRecorder(10)
	)

	BeforeAll(func() {
		ctx, cancel = context.WithCancel(context.TODO())
		k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme: scheme.Scheme,
		})
		Expect(err).ToNot(HaveOccurred())

		err = (&StaticGatewayConfigurationReconciler{
			Client:   k8sManager.GetClient(),
			Recorder: recorder,
		}).SetupWithManager(k8sManager)
		Expect(err).ToNot(HaveOccurred())
		go func() {
			defer GinkgoRecover()
			err = k8sManager.Start(ctx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).ToNot(HaveOccurred())

		gwConfig = &egressgatewayv1alpha1.StaticGatewayConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
				GatewayNodepoolName: "testgw",
				GatewayVMSSProfile: egressgatewayv1alpha1.GatewayVMSSProfile{
					VMSSResourceGroup:  "vmssRG",
					VMSSName:           "vmss",
					PublicIpPrefixSize: 31,
				},
				PublicIpPrefixId: "testPipPrefix",
			},
		}
		Expect(k8sClient.Create(ctx, gwConfig)).ToNot(HaveOccurred())
	})

	AfterAll(func() {
		Expect(k8sClient.Delete(ctx, namespace)).ToNot(HaveOccurred())
		cancel()
	})

	Context("new StaticGatewayConfiguration", func() {
		It("should create a new secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), secret)
			}, timeout, interval).ShouldNot(HaveOccurred())
			updatedGWConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updatedGWConfig)
			}, timeout, interval).ShouldNot(HaveOccurred())

			Expect(len(secret.Data)).To(Equal(2))
			privateKeyBytes, ok := secret.Data[consts.WireguardSecretKeyName]
			Expect(ok).To(BeTrue())
			Expect(privateKeyBytes).NotTo(BeEmpty())

			owner := metav1.GetControllerOf(secret)
			Expect(owner).NotTo(BeNil())
			Expect(owner.Name).To(Equal(testName))

			Expect(updatedGWConfig.Status.WireguardPrivateKeySecretRef).NotTo(BeNil())
			Expect(updatedGWConfig.Status.WireguardPrivateKeySecretRef.Name).To(Equal(testName))

			wgPrivateKey, err := wgtypes.ParseKey(string(privateKeyBytes))
			Expect(err).To(BeNil())
			wgPublicKey := wgPrivateKey.PublicKey().String()
			Expect(updatedGWConfig.Status.WireguardPublicKey).To(Equal(wgPublicKey))
		})

		It("should create a new lbconfig", func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig)
			}, timeout, interval).ShouldNot(HaveOccurred())
			Expect(lbConfig.Spec.GatewayNodepoolName).To(BeEquivalentTo(gwConfig.Spec.GatewayNodepoolName))
			Expect(lbConfig.Spec.GatewayVMSSProfile).To(BeEquivalentTo(gwConfig.Spec.GatewayVMSSProfile))

			owner := metav1.GetControllerOf(lbConfig)
			Expect(owner).NotTo(BeNil())
			Expect(owner.Name).To(Equal(testName))

			updatedGWConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updatedGWConfig)
			}, timeout, interval).ShouldNot(HaveOccurred())
			Expect(updatedGWConfig.Status.WireguardServerIP).To(BeEmpty())
			Expect(updatedGWConfig.Status.WireguardServerPort).To(BeZero())
		})
	})

	Context("update lbConfiguration", func() {
		BeforeAll(func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig)).ToNot(HaveOccurred())
			lbConfig.Status = &egressgatewayv1alpha1.GatewayLBConfigurationStatus{
				FrontendIP:     "1.1.1.1",
				ServerPort:     6000,
				PublicIpPrefix: "1.2.3.4/31",
			}
			Expect(k8sClient.Status().Update(ctx, lbConfig)).ToNot(HaveOccurred())
		})

		It("should update gwConfig status accordingly", func() {
			updatedGWConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
			Eventually(func() (map[string]interface{}, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updatedGWConfig); err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"ip":     updatedGWConfig.Status.WireguardServerIP,
					"port":   updatedGWConfig.Status.WireguardServerPort,
					"prefix": updatedGWConfig.Status.PublicIpPrefix,
				}, nil
			}, timeout, interval).Should(BeEquivalentTo(map[string]interface{}{
				"ip":     "1.1.1.1",
				"port":   int32(6000),
				"prefix": "1.2.3.4/31",
			}))
		})
	})

	Context("update gwConfiguration", func() {
		BeforeAll(func() {
			updateGWConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updateGWConfig)).ToNot(HaveOccurred())
			updateGWConfig.Spec.GatewayNodepoolName = "testgw1"
			updateGWConfig.Spec.GatewayVMSSProfile = egressgatewayv1alpha1.GatewayVMSSProfile{
				VMSSResourceGroup:  "vmssRG1",
				VMSSName:           "vmss1",
				PublicIpPrefixSize: 30,
			}
			Expect(k8sClient.Update(ctx, updateGWConfig)).ToNot(HaveOccurred())
		})

		It("should update lbConfig spec accordingly", func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Eventually(func() (map[string]interface{}, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig); err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"np":   lbConfig.Spec.GatewayNodepoolName,
					"rg":   lbConfig.Spec.VMSSResourceGroup,
					"vmss": lbConfig.Spec.VMSSName,
					"size": lbConfig.Spec.PublicIpPrefixSize,
				}, nil
			}, timeout, interval).Should(BeEquivalentTo(map[string]interface{}{
				"np":   "testgw1",
				"rg":   "vmssRG1",
				"vmss": "vmss1",
				"size": int32(30),
			}))
		})

		It("should generate events", func() {
			Eventually(len(recorder.Events), timeout, interval).Should(BeNumerically(">=", 3))
		})
	})

	Context("gateway profile config is deleted", func() {
		BeforeAll(func() {
			Expect(k8sClient.Delete(ctx, gwConfig, client.PropagationPolicy(metav1.DeletePropagationForeground))).ToNot(HaveOccurred())
		})

		It("should delete the new secret", func() {
			secret := &corev1.Secret{}
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testName}, secret)
				if err == nil {
					return false
				}
				GinkgoWriter.Println(err.Error())
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue())
		})

		It("should delete a new lbconfig", func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Eventually(func() bool {
				return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: testName}, lbConfig))
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func getResource(cl client.Client, object client.Object) error {
	key := types.NamespacedName{
		Name:      testName,
		Namespace: testNamespace,
	}
	err := cl.Get(context.TODO(), key, object)
	return err
}
