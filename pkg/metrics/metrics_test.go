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
