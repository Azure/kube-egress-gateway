// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"errors"
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
				GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
					VmssResourceGroup:  "vmssRG",
					VmssName:           "vmss",
					PublicIpPrefixSize: 31,
				},
				PublicIpPrefixId:   "testPipPrefix",
				ProvisionPublicIps: true,
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
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updatedGWConfig); err != nil {
					return err
				}
				if updatedGWConfig.Status.PrivateKeySecretRef == nil ||
					updatedGWConfig.Status.PrivateKeySecretRef.Name != testName {
					return errors.New("PrivateKeySecretRef is not ready yet")
				}
				return nil
			}, timeout, interval).ShouldNot(HaveOccurred())

			Expect(len(secret.Data)).To(Equal(2))
			privateKeyBytes, ok := secret.Data[consts.WireguardPrivateKeyName]
			Expect(ok).To(BeTrue())
			Expect(privateKeyBytes).NotTo(BeEmpty())

			owner := metav1.GetControllerOf(secret)
			Expect(owner).NotTo(BeNil())
			Expect(owner.Name).To(Equal(testName))

			wgPrivateKey, err := wgtypes.ParseKey(string(privateKeyBytes))
			Expect(err).To(BeNil())
			wgPublicKey := wgPrivateKey.PublicKey().String()
			Expect(updatedGWConfig.Status.PublicKey).To(Equal(wgPublicKey))
		})

		It("should create a new lbconfig", func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig)
			}, timeout, interval).ShouldNot(HaveOccurred())
			Expect(lbConfig.Spec.GatewayNodepoolName).To(BeEquivalentTo(gwConfig.Spec.GatewayNodepoolName))
			Expect(lbConfig.Spec.GatewayVmssProfile).To(BeEquivalentTo(gwConfig.Spec.GatewayVmssProfile))
			Expect(lbConfig.Spec.ProvisionPublicIps).To(BeEquivalentTo(gwConfig.Spec.ProvisionPublicIps))
			Expect(lbConfig.Spec.PublicIpPrefixId).To(BeEquivalentTo(gwConfig.Spec.PublicIpPrefixId))

			owner := metav1.GetControllerOf(lbConfig)
			Expect(owner).NotTo(BeNil())
			Expect(owner.Name).To(Equal(testName))

			updatedGWConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), updatedGWConfig)
			}, timeout, interval).ShouldNot(HaveOccurred())
			Expect(updatedGWConfig.Status.Ip).To(BeEmpty())
			Expect(updatedGWConfig.Status.Port).To(BeZero())
		})
	})

	Context("update lbConfiguration", func() {
		BeforeAll(func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig)).ToNot(HaveOccurred())
			lbConfig.Status = &egressgatewayv1alpha1.GatewayLBConfigurationStatus{
				FrontendIp:     "1.1.1.1",
				ServerPort:     6000,
				EgressIpPrefix: "1.2.3.4/31",
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
					"ip":     updatedGWConfig.Status.Ip,
					"port":   updatedGWConfig.Status.Port,
					"prefix": updatedGWConfig.Status.EgressIpPrefix,
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
			updateGWConfig.Spec.GatewayVmssProfile = egressgatewayv1alpha1.GatewayVmssProfile{}
			updateGWConfig.Spec.ProvisionPublicIps = false
			updateGWConfig.Spec.PublicIpPrefixId = ""
			Expect(k8sClient.Update(ctx, updateGWConfig)).ToNot(HaveOccurred())
		})

		It("should update lbConfig spec accordingly", func() {
			lbConfig := &egressgatewayv1alpha1.GatewayLBConfiguration{}
			Eventually(func() (map[string]interface{}, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gwConfig), lbConfig); err != nil {
					return nil, err
				}
				return map[string]interface{}{
					"np":        lbConfig.Spec.GatewayNodepoolName,
					"rg":        lbConfig.Spec.VmssResourceGroup,
					"vmss":      lbConfig.Spec.VmssName,
					"size":      lbConfig.Spec.PublicIpPrefixSize,
					"prefixId":  lbConfig.Spec.PublicIpPrefixId,
					"provision": lbConfig.Spec.ProvisionPublicIps,
				}, nil
			}, timeout, interval).Should(BeEquivalentTo(map[string]interface{}{
				"np":        "testgw1",
				"rg":        "",
				"vmss":      "",
				"size":      int32(0),
				"prefixId":  "",
				"provision": false,
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

var _ = Describe("test staticGatewayConfiguration validation", func() {
	var (
		gwConfig *egressgatewayv1alpha1.StaticGatewayConfiguration
	)

	BeforeEach(func() {
		gwConfig = &egressgatewayv1alpha1.StaticGatewayConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
			Spec: egressgatewayv1alpha1.StaticGatewayConfigurationSpec{
				GatewayVmssProfile: egressgatewayv1alpha1.GatewayVmssProfile{
					VmssResourceGroup:  "vmssRG",
					VmssName:           "vmss",
					PublicIpPrefixSize: 31,
				},
				PublicIpPrefixId:   "testPipPrefix",
				ProvisionPublicIps: true,
			},
		}
	})

	Context("validate GatewayNodepoolName", func() {
		It("should pass when only GatewayNodepoolName is provided", func() {
			gwConfig.Spec.GatewayNodepoolName = "testgw"
			gwConfig.Spec.GatewayVmssProfile = egressgatewayv1alpha1.GatewayVmssProfile{}
			err := validate(gwConfig)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should fail when both GatewayNodepoolName and GatewayVmssProfile are provided", func() {
			gwConfig.Spec.GatewayNodepoolName = "testgw"
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})

		It("should pass when GatewayNodepoolName is not provided but GatewayVmssProfile is provided", func() {
			err := validate(gwConfig)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should fail when neither GatewayNodepoolName nor GatewayVmssProfile is provided", func() {
			gwConfig.Spec.GatewayVmssProfile = egressgatewayv1alpha1.GatewayVmssProfile{}
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("validate GatewayVmssProfile", func() {
		It("should fail when VmssResourceGroup is not provided", func() {
			gwConfig.Spec.GatewayVmssProfile.VmssResourceGroup = ""
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})

		It("should fail when VmssName is not provided", func() {
			gwConfig.Spec.GatewayVmssProfile.VmssName = ""
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})

		It("should fail when PublicIpPrefixSize < 0", func() {
			gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize = -1
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})

		It("should fail when PublicIpPrefixSize > 31", func() {
			gwConfig.Spec.GatewayVmssProfile.PublicIpPrefixSize = 32
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
		})
	})

	Context("validate publicIpPrefix provision", func() {
		It("should fail when PublicIPPrefixId is provided but ProvisionPublicIps is false", func() {
			gwConfig.Spec.ProvisionPublicIps = false
			err := validate(gwConfig)
			Expect(err).Should(HaveOccurred())
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
