// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.package metrics
package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

func TestNewMetricsContext(t *testing.T) {
	namespace := "testns"
	operation := "operation"
	subscriptionID := "subscriptionID"
	resourceGroup := "resourceGroup"
	resource := "ns/name"
	mc := NewMetricsContext(namespace, operation, subscriptionID, resourceGroup, resource)
	assert.WithinDuration(t, mc.start, time.Now(), 2*time.Second)
	assert.Equal(t, []string{namespace, operation, subscriptionID, resourceGroup, resource}, mc.labels)
}

func TestObserveControllerReconcileMetrics(t *testing.T) {
	namespace := "testns"
	operation := "operation"
	subscriptionID := "subID"
	resourceGroup := "rg"
	resource := "ns/name"

	failCountMeta := `
		# HELP controller_reconcile_fail_count Number of failed static egress gateway controller reconciliations
		# TYPE controller_reconcile_fail_count counter
`
	tests := []struct {
		name                 string
		succeeded            bool
		expectedFailCount    int
		expectedLatencyCount int
		expectedCounter      string
	}{
		{
			name:                 "should generate metrics for successful reconcile",
			succeeded:            true,
			expectedFailCount:    0,
			expectedLatencyCount: 1,
			expectedCounter:      "",
		},
		{
			name:                 "should generate metrics for failed reconcile",
			succeeded:            false,
			expectedFailCount:    1,
			expectedLatencyCount: 1,
			expectedCounter: `
			controller_reconcile_fail_count{namespace="testns",operation="operation",resource="ns/name",resource_group="rg",subscription_id="subID"} 1
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mc := NewMetricsContext(namespace, operation, subscriptionID, resourceGroup, resource)
			mc.ObserveControllerReconcileMetrics(test.succeeded)

			failCount := testutil.CollectAndCount(ControllerReconcileFailCount)
			assert.Equal(t, test.expectedFailCount, failCount)
			latencyCount := testutil.CollectAndCount(ControllerReconcileLatency)
			assert.Equal(t, test.expectedLatencyCount, latencyCount)

			assert.Nil(t, testutil.CollectAndCompare(ControllerReconcileFailCount, strings.NewReader(failCountMeta+test.expectedCounter)))

			ControllerReconcileFailCount.Reset()
			ControllerReconcileLatency.Reset()
		})
	}
}

func TestObserve(t *testing.T) {
	namespace := "testns"
	operation := "operation"
	subscriptionID := "subID"
	resourceGroup := "rg"
	resource := "ns/name"

	defer func() {
		ControllerReconcileFailCount.Reset()
		ControllerReconcileLatency.Reset()
	}()

	LatencyMeta := `
		# HELP controller_reconcile_latency Latency of static egress gateway controller reconciliations
		# TYPE controller_reconcile_latency histogram
`
	LatencyData := `
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="0.1"} 1
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="0.2"} 1
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="0.5"} 1
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="1"} 1
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="5"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="10"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="15"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="20"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="30"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="40"} 2
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="50"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="60"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="100"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="200"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="300"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="600"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="1200"} 3
		controller_reconcile_latency_bucket{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID",le="+Inf"} 4
		controller_reconcile_latency_sum{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID"} 1250.05
		controller_reconcile_latency_count{namespace="testns",operation="operation",resource_group="rg",subscription_id="subID"} 4
`
	mc := NewMetricsContext(namespace, operation, subscriptionID, resourceGroup, resource)
	mc.observe(0.05)
	mc.observe(3.0)
	mc.observe(42.0)
	mc.observe(1205.0)
	assert.Equal(t, 1, testutil.CollectAndCount(ControllerReconcileLatency))
	assert.Nil(t, testutil.CollectAndCompare(ControllerReconcileLatency, strings.NewReader(LatencyMeta+LatencyData)))
}

func TestCNIManagerPodEndpointOperationFailCount(t *testing.T) {
	defer CNIManagerPodEndpointOperationFailCount.Reset()

	namespace := "testns"
	operation := "create_or_update"
	pod := "test-pod"

	meta := `
		# HELP cnimanager_podendpoint_operation_failures_total Number of failed PodEndpoint operations in CNI manager
		# TYPE cnimanager_podendpoint_operation_failures_total counter
`

	tests := []struct {
		name            string
		namespace       string
		operation       string
		pod             string
		incrementCount  int
		expectedCounter string
	}{
		{
			name:           "should increment counter for create_or_update operation",
			namespace:      namespace,
			operation:      operation,
			pod:            pod,
			incrementCount: 1,
			expectedCounter: `
			cnimanager_podendpoint_operation_failures_total{namespace="testns",operation="create_or_update",pod="test-pod"} 1
`,
		},
		{
			name:           "should increment counter for delete operation",
			namespace:      namespace,
			operation:      "delete",
			pod:            pod,
			incrementCount: 2,
			expectedCounter: `
			cnimanager_podendpoint_operation_failures_total{namespace="testns",operation="delete",pod="test-pod"} 2
`,
		},
		{
			name:           "should track multiple pods separately",
			namespace:      namespace,
			operation:      operation,
			pod:            "another-pod",
			incrementCount: 3,
			expectedCounter: `
			cnimanager_podendpoint_operation_failures_total{namespace="testns",operation="create_or_update",pod="another-pod"} 3
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			CNIManagerPodEndpointOperationFailCount.Reset()

			for i := 0; i < test.incrementCount; i++ {
				CNIManagerPodEndpointOperationFailCount.WithLabelValues(
					test.namespace,
					test.operation,
					test.pod,
				).Inc()
			}

			count := testutil.CollectAndCount(CNIManagerPodEndpointOperationFailCount)
			assert.Equal(t, 1, count)
			assert.Nil(t, testutil.CollectAndCompare(CNIManagerPodEndpointOperationFailCount, strings.NewReader(meta+test.expectedCounter)))
		})
	}
}

func TestCNIManagerConfigOperationFailCount(t *testing.T) {
	defer CNIManagerConfigOperationFailCount.Reset()

	meta := `
		# HELP cnimanager_cni_config_operation_failures_total Number of failed CNI configuration operations
		# TYPE cnimanager_cni_config_operation_failures_total counter
`

	tests := []struct {
		name            string
		operation       string
		incrementCount  int
		expectedCounter string
	}{
		{
			name:           "should increment counter for install operation",
			operation:      "install",
			incrementCount: 1,
			expectedCounter: `
			cnimanager_cni_config_operation_failures_total{operation="install"} 1
`,
		},
		{
			name:           "should increment counter for regenerate operation",
			operation:      "regenerate",
			incrementCount: 5,
			expectedCounter: `
			cnimanager_cni_config_operation_failures_total{operation="regenerate"} 5
`,
		},
		{
			name:           "should increment counter for uninstall operation",
			operation:      "uninstall",
			incrementCount: 2,
			expectedCounter: `
			cnimanager_cni_config_operation_failures_total{operation="uninstall"} 2
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			CNIManagerConfigOperationFailCount.Reset()

			for i := 0; i < test.incrementCount; i++ {
				CNIManagerConfigOperationFailCount.WithLabelValues(test.operation).Inc()
			}

			count := testutil.CollectAndCount(CNIManagerConfigOperationFailCount)
			assert.Equal(t, 1, count)
			assert.Nil(t, testutil.CollectAndCompare(CNIManagerConfigOperationFailCount, strings.NewReader(meta+test.expectedCounter)))
		})
	}
}

func TestCNIManagerNodeTaintOperationFailCount(t *testing.T) {
	defer CNIManagerNodeTaintOperationFailCount.Reset()

	meta := `
		# HELP cnimanager_node_taint_operation_failures_total Number of failed node taint removal operations
		# TYPE cnimanager_node_taint_operation_failures_total counter
`

	tests := []struct {
		name            string
		node            string
		incrementCount  int
		expectedCounter string
	}{
		{
			name:           "should increment counter for single node",
			node:           "node-1",
			incrementCount: 1,
			expectedCounter: `
			cnimanager_node_taint_operation_failures_total{node="node-1"} 1
`,
		},
		{
			name:           "should increment counter for multiple failures on same node",
			node:           "node-2",
			incrementCount: 3,
			expectedCounter: `
			cnimanager_node_taint_operation_failures_total{node="node-2"} 3
`,
		},
		{
			name:           "should track different nodes separately",
			node:           "node-3",
			incrementCount: 2,
			expectedCounter: `
			cnimanager_node_taint_operation_failures_total{node="node-3"} 2
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			CNIManagerNodeTaintOperationFailCount.Reset()

			for i := 0; i < test.incrementCount; i++ {
				CNIManagerNodeTaintOperationFailCount.WithLabelValues(test.node).Inc()
			}

			count := testutil.CollectAndCount(CNIManagerNodeTaintOperationFailCount)
			assert.Equal(t, 1, count)
			assert.Nil(t, testutil.CollectAndCompare(CNIManagerNodeTaintOperationFailCount, strings.NewReader(meta+test.expectedCounter)))
		})
	}
}

func TestCNIManagerMetricsIntegration(t *testing.T) {
	defer func() {
		CNIManagerPodEndpointOperationFailCount.Reset()
		CNIManagerConfigOperationFailCount.Reset()
		CNIManagerNodeTaintOperationFailCount.Reset()
	}()

	// Simulate a series of operations with failures
	CNIManagerPodEndpointOperationFailCount.WithLabelValues("default", "create_or_update", "pod-1").Inc()
	CNIManagerPodEndpointOperationFailCount.WithLabelValues("default", "delete", "pod-2").Inc()
	CNIManagerConfigOperationFailCount.WithLabelValues("install").Inc()
	CNIManagerConfigOperationFailCount.WithLabelValues("regenerate").Inc()
	CNIManagerConfigOperationFailCount.WithLabelValues("regenerate").Inc()
	CNIManagerNodeTaintOperationFailCount.WithLabelValues("node-1").Inc()

	// Verify all metrics are collected
	podEndpointCount := testutil.CollectAndCount(CNIManagerPodEndpointOperationFailCount)
	assert.Equal(t, 2, podEndpointCount, "should have 2 distinct PodEndpoint metric series")

	configCount := testutil.CollectAndCount(CNIManagerConfigOperationFailCount)
	assert.Equal(t, 2, configCount, "should have 2 distinct Config metric series")

	nodeTaintCount := testutil.CollectAndCount(CNIManagerNodeTaintOperationFailCount)
	assert.Equal(t, 1, nodeTaintCount, "should have 1 distinct NodeTaint metric series")
}
