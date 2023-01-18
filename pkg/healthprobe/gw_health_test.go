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
package healthprobe

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGatewayHealthServer(t *testing.T) {
	assert.Empty(t, activeGateways, "active gateway map should be empty at the beginning")

	// test unexpected request path
	testHandler("", http.StatusBadRequest, t)
	testHandler("/gw", http.StatusBadRequest, t)
	testHandler("/gw/123/123", http.StatusBadRequest, t)

	// Add gateway
	err := AddGateway("123")
	assert.Nil(t, err, "AddGateway should not report error")
	assert.Equal(t, 1, len(activeGateways), "active gateway map should have 1 element")
	testHandler("/gw/123", http.StatusOK, t)
	testHandler("/gw/456", http.StatusServiceUnavailable, t)

	// Remove gateway
	err = RemoveGateway("456")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Equal(t, 1, len(activeGateways), "active gateway map should have 1 element")
	err = RemoveGateway("123")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Empty(t, activeGateways, "active gateway map should be empty")
	testHandler("/gw/123", http.StatusServiceUnavailable, t)
	testHandler("/gw/456", http.StatusServiceUnavailable, t)

	// Add multiple gateways
	err = AddGateway("abc")
	assert.Nil(t, err, "AddGateway should not report error")
	err = AddGateway("def")
	assert.Nil(t, err, "AddGateway should not report error")
	err = AddGateway("ghi")
	assert.Nil(t, err, "AddGateway should not report error")
	assert.Equal(t, 3, len(activeGateways), "active gateway map should have 3 elements")
	testHandler("/gw/def", http.StatusOK, t)
	testHandler("/gw/xyz", http.StatusServiceUnavailable, t)

	// Delete multiple gateways
	err = RemoveGateway("def")
	assert.Nil(t, err, "RemoveGateway should not report error")
	err = RemoveGateway("abc")
	assert.Nil(t, err, "RemoveGateway should not report error")
	assert.Equal(t, 1, len(activeGateways), "active gateway map should have 1 element")
	testHandler("/gw/abc", http.StatusServiceUnavailable, t)
	testHandler("/gw/ghi", http.StatusOK, t)
}

func testHandler(requestPath string, status int, t *testing.T) {
	req, err := http.NewRequest("GET", requestPath, nil)
	assert.Nil(t, err, "testHandler: failed to create new http request")
	resp := httptest.NewRecorder()

	serveHTTP(resp, req)
	assert.Equal(t, status, resp.Code, "testHandler: got unexpected http status code")
}
