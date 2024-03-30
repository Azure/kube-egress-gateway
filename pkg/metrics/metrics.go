// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	ControllerReconcileFailCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "controller_reconcile_fail_count",
			Help: "Number of failed static egress gateway controller reconciliations",
		},
		[]string{"namespace", "operation", "subscription_id", "resource_group", "resource"},
	)

	ControllerReconcileLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "controller_reconcile_latency",
			Help:    "Latency of static egress gateway controller reconciliations",
			Buckets: []float64{0.1, 0.2, 0.5, 1, 5, 10, 15, 20, 30, 40, 50, 60, 100, 200, 300, 600, 1200}, // seconds
		},
		[]string{"namespace", "operation", "subscription_id", "resource_group"},
	)
)

type MetricsContext struct {
	start  time.Time
	labels []string
}

func NewMetricsContext(namespace, operation, subscriptionID, resourceGroup, resource string) *MetricsContext {
	return &MetricsContext{
		start:  time.Now(),
		labels: []string{namespace, operation, subscriptionID, resourceGroup, resource},
	}
}

func (mc *MetricsContext) ObserveControllerReconcileMetrics(succeeded bool) {
	if !succeeded {
		ControllerReconcileFailCount.WithLabelValues(mc.labels...).Inc()
	} else {
		// reset FailCount metrics if reconcile is successful
		// DeleteLabelValues does not return error is label values are not found
		_ = ControllerReconcileFailCount.DeleteLabelValues(mc.labels...)
	}
	latency := time.Since(mc.start).Seconds()
	mc.observe(latency)
}

func (mc *MetricsContext) observe(latency float64) {
	// trim the last "resource" label
	ControllerReconcileLatency.WithLabelValues(mc.labels[:4]...).Observe(latency)
}
