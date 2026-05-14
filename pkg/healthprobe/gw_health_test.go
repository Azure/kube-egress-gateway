// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package healthprobe

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGatewayHealthServer(t *testing.T) {
	svr := NewLBProbeServer(1000, 0)
	assert.Empty(t, svr.GetGateways(), "active gateway map should be empty at the beginning")

	// test unexpected request path
	testHandler(svr, "", http.StatusBadRequest, t)
	testHandler(svr, "/gw", http.StatusBadRequest, t)
	testHandler(svr, "/gw/123/123", http.StatusBadRequest, t)

	// Add gateway
	err := svr.AddGateway("123")
	assert.Nil(t, err, "AddGateway should not report error")
	assert.Equal(t, 1, len(svr.GetGateways()), "active gateway map should have 1 element")
	testHandler(svr, "/gw/123", http.StatusOK, t)
	testHandler(svr, "/gw/456", http.StatusServiceUnavailable, t)

	// Remove gateway
	err = svr.RemoveGateway("456")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Equal(t, 1, len(svr.GetGateways()), "active gateway map should have 1 element")
	err = svr.RemoveGateway("123")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Empty(t, svr.GetGateways(), "active gateway map should be empty")
	testHandler(svr, "/gw/123", http.StatusServiceUnavailable, t)
	testHandler(svr, "/gw/456", http.StatusServiceUnavailable, t)

	// Add multiple gateways
	err = svr.AddGateway("abc")
	assert.Nil(t, err, "AddGateway should not report error")
	err = svr.AddGateway("def")
	assert.Nil(t, err, "AddGateway should not report error")
	err = svr.AddGateway("ghi")
	assert.Nil(t, err, "AddGateway should not report error")
	assert.Equal(t, 3, len(svr.GetGateways()), "active gateway map should have 3 elements")
	testHandler(svr, "/gw/def", http.StatusOK, t)
	testHandler(svr, "/gw/xyz", http.StatusServiceUnavailable, t)

	// Delete multiple gateways
	err = svr.RemoveGateway("def")
	assert.Nil(t, err, "RemoveGateway should not report error")
	err = svr.RemoveGateway("abc")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Equal(t, 1, len(svr.GetGateways()), "active gateway map should have 1 element")
	testHandler(svr, "/gw/abc", http.StatusServiceUnavailable, t)
	testHandler(svr, "/gw/ghi", http.StatusOK, t)
}

func testHandler(svr *LBProbeServer, requestPath string, status int, t *testing.T) {
	req, err := http.NewRequest("GET", requestPath, nil)
	assert.Nil(t, err, "testHandler: failed to create new http request")
	resp := httptest.NewRecorder()

	svr.serveHTTP(resp, req)
	assert.Equal(t, status, resp.Code, "testHandler: got unexpected http status code")
}

func TestGatewayHealthServer_ShutdownReturnsUnavailable(t *testing.T) {
	svr := NewLBProbeServer(1000, 0)

	err := svr.AddGateway("123")
	assert.Nil(t, err)

	// Active gateway should return 200
	testHandler(svr, "/gw/123", http.StatusOK, t)

	// Simulate shutdown
	svr.lock.Lock()
	svr.shuttingDown = true
	svr.lock.Unlock()

	// Same gateway should now return 503
	testHandler(svr, "/gw/123", http.StatusServiceUnavailable, t)
}

func TestGatewayHealthServer_GracefulShutdownLifecycle(t *testing.T) {
	// Use port 0 to let the OS pick a free port
	svr := NewLBProbeServer(0, 0)
	err := svr.AddGateway("test-gw")
	assert.Nil(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the server; capture the actual listen address
	listener, err := net.Listen("tcp", ":0")
	assert.Nil(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	svr.listenPort = port

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- svr.Start(ctx)
	}()

	// Wait for server to be ready
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	assert.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/gw/test-gw")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 3*time.Second, 50*time.Millisecond, "server should become ready and return 200")

	// Cancel context (simulates SIGTERM)
	cancel()

	// The server should still be running during lbProbeDrainDelaySeconds,
	// but now returning 503 for all gateways
	assert.Eventually(t, func() bool {
		resp, err := http.Get(baseURL + "/gw/test-gw")
		if err != nil {
			return false // server not yet processing the shutdown
		}
		resp.Body.Close()
		return resp.StatusCode == http.StatusServiceUnavailable
	}, 3*time.Second, 50*time.Millisecond, "server should return 503 during shutdown drain")

	// Server should eventually shut down
	select {
	case err := <-serverDone:
		assert.Nil(t, err, "server should shut down without error")
	case <-time.After(DefaultLBProbeDrainDelaySeconds + 10*time.Second):
		t.Fatal("server did not shut down within expected time")
	}
}
