// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package healthprobe

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGatewayHealthServer(t *testing.T) {
	svr := NewLBProbeServer(1000)
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
