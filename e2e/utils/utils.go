// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package utils

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/Azure/kube-egress-gateway/api/v1alpha1"
)

const (
	poll                    = 2 * time.Second
	pollTimeout             = 5 * time.Minute
	pollTimeoutForProvision = 30 * time.Minute
	nodeLocationLabel       = "failure-domain.beta.kubernetes.io/region"

	AzureClientIDEnv           = "AZURE_CLIENT_ID"
	AzureClientSecretEnv       = "AZURE_CLIENT_SECRET"
	ManagedIdentityClientIDEnv = "AZURE_MANAGED_IDENTITY_CLIENT_ID"
	SubscriptionIDEnv          = "AZURE_SUBSCRIPTION_ID"
	TenantIDEnv                = "AZURE_TENANT_ID"
)

var (
	vmssVMProviderIDRE = regexp.MustCompile(`azure:///subscriptions/(?:.*)/resourceGroups/(.+)/providers/Microsoft.Compute/virtualMachineScaleSets/(.+)/virtualMachines/(?:\d+)`)
)

func CreateK8sClient() (k8sClient client.Client, podLogClient clientset.Interface, err error) {
	apischeme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(apischeme))
	utilruntime.Must(v1alpha1.AddToScheme(apischeme))
	restConfig := config.GetConfigOrDie()
	k8sClient, err = client.New(restConfig, client.Options{Scheme: apischeme})
	if err != nil {
		return
	}
	podLogClient, err = clientset.NewForConfig(restConfig)
	return
}

func CreateAzureClients() (azclient.ClientFactory, error) {
	var subscriptionID, tenantID, clientID, clientSecret, managedIdentityClientID string
	if subscriptionID = os.Getenv(SubscriptionIDEnv); subscriptionID == "" {
		return nil, fmt.Errorf(SubscriptionIDEnv + " is not set")
	}
	if tenantID = os.Getenv(TenantIDEnv); tenantID == "" {
		return nil, fmt.Errorf(TenantIDEnv + " is not set")
	}

	armConfig := &azclient.ARMClientConfig{
		TenantID: tenantID,
		// only test in Public Cloud
		Cloud: "AZUREPUBLICCLOUD",
	}

	authConfig := &azclient.AzureAuthConfig{}
	clientID = os.Getenv(AzureClientIDEnv)
	clientSecret = os.Getenv(AzureClientSecretEnv)
	managedIdentityClientID = os.Getenv(ManagedIdentityClientIDEnv)
	if len(clientID) > 0 && len(clientSecret) > 0 {
		authConfig.AADClientID = clientID
		authConfig.AADClientSecret = clientSecret
	} else {
		authConfig.UseManagedIdentityExtension = true
		authConfig.UserAssignedIdentityID = managedIdentityClientID
	}
	authProvider, err := azclient.NewAuthProvider(armConfig, authConfig)
	if err != nil {
		return nil, err
	}
	clientOps, _, err := azclient.GetAzCoreClientOption(armConfig)
	if err != nil {
		return nil, err
	}
	return azclient.NewClientFactory(&azclient.ClientFactoryConfig{SubscriptionID: subscriptionID}, armConfig, clientOps.Cloud, authProvider.GetAzIdentity())
}

func CreateNamespace(namespaceName string, c client.Client) error {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespaceName,
			Namespace: "",
		},
	}
	return CreateK8sObject(namespaceObj, c)
}

func DeleteNamespace(namespaceName string, c client.Client) error {
	namespaceObj := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespaceName,
			Namespace: "",
		},
	}
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Delete(ctx, namespaceObj)
		if err != nil {
			if retriable(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

func CreateK8sObject(obj client.Object, c client.Client) error {
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Create(ctx, obj)
		if err != nil {
			if retriable(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

func WaitStaticGatewayProvision(sgw *v1alpha1.StaticGatewayConfiguration, c client.Client) (string, error) {
	pipPrefix := ""
	key := types.NamespacedName{
		Name:      sgw.Name,
		Namespace: sgw.Namespace,
	}
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeoutForProvision, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, key, sgw)
		if err != nil {
			if retriable(err) {
				return false, nil
			}
			return false, err
		}
		if len(sgw.Status.EgressIpPrefix) > 0 {
			pipPrefix = sgw.Status.EgressIpPrefix
			return true, nil
		}
		return false, nil
	}); err != nil {
		return "", err
	}
	return pipPrefix, nil
}

func WaitStaticGatewayDeletion(sgw *v1alpha1.StaticGatewayConfiguration, c client.Client) error {
	key := types.NamespacedName{
		Name:      sgw.Name,
		Namespace: sgw.Namespace,
	}
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeoutForProvision, true, func(ctx context.Context) (bool, error) {
		err := c.Delete(ctx, sgw)
		if err != nil {
			if retriable(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return err
	}
	// Wait until gatewayVMConfig is deleted
	gatewayVMConfig := &v1alpha1.GatewayVMConfiguration{}
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeoutForProvision, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, key, gatewayVMConfig)
		if err != nil {
			if retriable(err) {
				return false, nil
			}
			if apierrs.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		return false, nil
	}); err != nil {
		return err
	}
	return nil
}

func WaitPipPrefixDeletion(resourceGroup, pipName string, c publicipprefixclient.Interface) error {
	if err := wait.PollUntilContextTimeout(context.Background(), poll, pollTimeoutForProvision, true, func(ctx context.Context) (bool, error) {
		err := c.Delete(ctx, resourceGroup, pipName)
		if err != nil {
			// retry when pip prefix is still in use
			if strings.Contains(err.Error(), "InUsePublicIpPrefixCannotBeDeleted") {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}); err != nil {
		return err
	}
	return nil
}

func GetGatewayVmssProfile(c client.Client) (resourceGroup, vmssName, location string, prefixLen int32, err error) {
	nodes := &corev1.NodeList{}
	err = c.List(context.Background(), nodes, client.MatchingLabels{"kubeegressgateway.azure.com/mode": "true"})
	if err != nil {
		return
	}
	if len(nodes.Items) == 0 {
		err = fmt.Errorf("failed to find any gateway nodes")
		return
	}

	// At this moment, we only test one gateway nodepool
	matches := vmssVMProviderIDRE.FindStringSubmatch(nodes.Items[0].Spec.ProviderID)
	if len(matches) != 3 {
		err = fmt.Errorf("gateway node providerID (%s) is not valid", nodes.Items[0].Spec.ProviderID)
		return
	}

	nodeCount := len(nodes.Items)
	for i, k := 1, 1; k < nodeCount; i, k = i+1, k<<1 {
		prefixLen = int32(32 - i)
	}

	location = nodes.Items[0].Labels[nodeLocationLabel]
	return matches[1], matches[2], location, prefixLen, nil
}

func retriable(err error) bool {
	// possible transient errors.
	if apierrs.IsInternalError(err) || apierrs.IsTimeout(err) || apierrs.IsServerTimeout(err) ||
		apierrs.IsTooManyRequests(err) || utilnet.IsProbableEOF(err) || utilnet.IsConnectionReset(err) {
		return true
	}
	// error with Retry-After header.
	_, shouldRetry := apierrs.SuggestsClientDelay(err)
	return shouldRetry
}
