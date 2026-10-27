// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package manager

import (
	"context"
	"errors"
	"testing"

	network "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient/publicipprefixclient/mock_publicipprefixclient"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	egressgatewayv1alpha1 "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/utils/to"
)

type byoPrefixEventRecorder struct {
	*record.FakeRecorder
	object runtime.Object
}

func (r *byoPrefixEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	r.object = object
	r.FakeRecorder.Event(object, eventtype, reason, message)
}

func TestBYOPrefixAuthorization(t *testing.T) {
	tests := []struct {
		name        string
		enforce     bool
		tag         *string
		prefixSize  int32
		missingSGC  bool
		getErr      error
		wantErr     string
		wantWarning bool
	}{
		{name: "enforced missing tag", enforce: true, prefixSize: 31, wantErr: "is not authorized"},
		{name: "enforced wrong namespace", enforce: true, tag: to.Ptr("other-ns"), prefixSize: 31, wantErr: "is not authorized"},
		{name: "enforced authorized namespace", enforce: true, tag: to.Ptr("tenant"), prefixSize: 31},
		{name: "compatibility missing tag", prefixSize: 31, wantWarning: true},
		{name: "compatibility empty tag", tag: to.Ptr(""), prefixSize: 31, wantWarning: true},
		{name: "compatibility wrong namespace", tag: to.Ptr("other-ns"), prefixSize: 31, wantWarning: true},
		{name: "compatibility authorized namespace", tag: to.Ptr("tenant"), prefixSize: 31},
		{name: "compatibility wildcard", tag: to.Ptr("*"), prefixSize: 31},
		{name: "compatibility invalid prefix length", prefixSize: 30, wantErr: "invalid length", wantWarning: true},
		{name: "compatibility missing SGC", prefixSize: 31, missingSGC: true, wantErr: "failed to get StaticGatewayConfiguration"},
		{name: "compatibility Azure failure", getErr: errors.New("Azure request failed"), wantErr: "Azure request failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			az := getMockAzureManager(gomock.NewController(t))
			scheme := runtime.NewScheme()
			require.NoError(t, egressgatewayv1alpha1.AddToScheme(scheme))
			gwConfig := &egressgatewayv1alpha1.StaticGatewayConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "tenant", UID: "sgc-uid"},
			}
			builder := fake.NewClientBuilder().WithScheme(scheme)
			if !tt.missingSGC {
				builder.WithObjects(gwConfig)
			}
			recorder := &byoPrefixEventRecorder{FakeRecorder: record.NewFakeRecorder(10)}
			r := &GatewayVMConfigurationReconciler{
				Client: builder.Build(), AzureManager: az, Recorder: recorder,
				EnforceBYOPrefixAuthorization: tt.enforce,
			}
			prefixID := "/subscriptions/testSub/resourceGroups/rg/providers/Microsoft.Network/publicIPPrefixes/prefix"
			vmConfig := &egressgatewayv1alpha1.GatewayVMConfiguration{
				ObjectMeta: metav1.ObjectMeta{Name: gwConfig.Name, Namespace: gwConfig.Namespace},
				Spec: egressgatewayv1alpha1.GatewayVMConfigurationSpec{
					ProvisionPublicIps: true, PublicIpPrefixId: prefixID,
				},
			}
			prefix := &network.PublicIPPrefix{
				ID: to.Ptr(prefixID),
				Properties: &network.PublicIPPrefixPropertiesFormat{
					PrefixLength: to.Ptr(tt.prefixSize), IPPrefix: to.Ptr("1.2.3.4/31"),
				},
			}
			if tt.tag != nil {
				prefix.Tags = map[string]*string{consts.PublicIPPrefixAllowedNamespacesTagKey: tt.tag}
			}
			prefixClient := az.PublicIPPrefixClient.(*mock_publicipprefixclient.MockInterface)
			prefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(prefix, tt.getErr)
			cidr, id, managed, err := r.ensurePublicIPPrefix(context.Background(), 31, vmConfig)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				require.Empty(t, cidr)
				require.Empty(t, id)
				if tt.missingSGC {
					require.True(t, apierrors.IsNotFound(err))
				}
			} else {
				require.NoError(t, err)
				require.Equal(t, "1.2.3.4/31", cidr)
				require.Equal(t, prefixID, id)
			}
			require.False(t, managed)
			select {
			case event := <-recorder.Events:
				require.True(t, tt.wantWarning, "unexpected event: %s", event)
				require.Contains(t, event, corev1.EventTypeWarning+" BYOPrefixAuthorizationRequired")
				require.Contains(t, event, prefixID)
				require.Contains(t, event, "namespace tenant")
				require.Contains(t, event, consts.PublicIPPrefixAllowedNamespacesTagKey)
				require.Contains(t, event, "preserving other approved namespaces")
				require.Contains(t, event, "https://learn.microsoft.com/azure/aks/configure-static-egress-gateway#authorize-the-namespace-on-the-prefix")
				target, ok := recorder.object.(*egressgatewayv1alpha1.StaticGatewayConfiguration)
				require.True(t, ok, "warning must target the user-facing SGC")
				require.Equal(t, gwConfig.Name, target.Name)
				require.Equal(t, gwConfig.Namespace, target.Namespace)
				require.Equal(t, gwConfig.UID, target.UID)
			default:
				require.False(t, tt.wantWarning, "expected migration warning")
			}

			if tt.wantWarning && tt.wantErr == "" {
				prefix.Tags = map[string]*string{consts.PublicIPPrefixAllowedNamespacesTagKey: to.Ptr("tenant")}
				prefixClient.EXPECT().Get(gomock.Any(), "rg", "prefix", gomock.Any()).Return(prefix, nil)
				_, _, _, err = r.ensurePublicIPPrefix(context.Background(), 31, vmConfig)
				require.NoError(t, err)
				select {
				case event := <-recorder.Events:
					t.Fatalf("unexpected warning after tag correction: %s", event)
				default:
				}
			}
		})
	}
}
