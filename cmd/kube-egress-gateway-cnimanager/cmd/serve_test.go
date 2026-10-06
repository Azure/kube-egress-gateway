// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func TestProbeHandler(t *testing.T) {
	tests := []struct {
		name           string
		grpcServing    bool
		cniReady       bool
		expectedHealth int
		expectedReady  int
	}{
		{
			name:           "not serving",
			expectedHealth: http.StatusServiceUnavailable,
			expectedReady:  http.StatusServiceUnavailable,
		},
		{
			name:           "grpc serving but cni not ready",
			grpcServing:    true,
			expectedHealth: http.StatusOK,
			expectedReady:  http.StatusServiceUnavailable,
		},
		{
			name:           "cni ready but grpc not serving",
			cniReady:       true,
			expectedHealth: http.StatusServiceUnavailable,
			expectedReady:  http.StatusServiceUnavailable,
		},
		{
			name:           "ready",
			grpcServing:    true,
			cniReady:       true,
			expectedHealth: http.StatusOK,
			expectedReady:  http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			healthServer := health.NewServer()
			if !test.grpcServing {
				healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_NOT_SERVING)
			}
			handler := newProbeHandler(healthServer, func() bool {
				return test.cniReady
			})

			healthRecorder := httptest.NewRecorder()
			handler.ServeHTTP(healthRecorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if healthRecorder.Code != test.expectedHealth {
				t.Fatalf("unexpected health status: got %d, want %d", healthRecorder.Code, test.expectedHealth)
			}

			readyRecorder := httptest.NewRecorder()
			handler.ServeHTTP(readyRecorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
			if readyRecorder.Code != test.expectedReady {
				t.Fatalf("unexpected readiness status: got %d, want %d", readyRecorder.Code, test.expectedReady)
			}
		})
	}
}
