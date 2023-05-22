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
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/Azure/kube-egress-gateway/pkg/consts"
)

var (
	httpServer     *http.Server
	lock           sync.RWMutex
	activeGateways map[string]bool
)

func init() {
	mux := http.NewServeMux()
	mux.HandleFunc(consts.GatewayHealthProbeEndpoint, serveHTTP)

	httpServer = &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(int(consts.WireguardDaemonServicePort))),
		Handler:           mux,
		MaxHeaderBytes:    1 << 20,
		IdleTimeout:       90 * time.Second, // matches http.DefaultTransport keep-alive timeout
		ReadHeaderTimeout: 32 * time.Second,
	}

	activeGateways = make(map[string]bool)
}

func Start(ctx context.Context) error {
	log := log.FromContext(ctx)

	go func() {
		log.Info("Starting gateway lb health probe server")
		if err := httpServer.ListenAndServe(); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			log.Error(err, "failed to start gateway lb health probe server")
		}
	}()

	// Shutdown the server when stop is closed.
	<-ctx.Done()
	log.Info("Stopping gateway lb health probe server")
	if err := httpServer.Close(); err != nil {
		log.Error(err, "failed to close gateway lb health probe server")
		return err
	}
	return nil
}

func AddGateway(gatewayUID string) error {
	lock.Lock()
	defer lock.Unlock()

	activeGateways[gatewayUID] = true
	return nil
}

func RemoveGateway(gatewayUID string) error {
	lock.Lock()
	defer lock.Unlock()

	delete(activeGateways, gatewayUID)
	return nil
}

func serveHTTP(resp http.ResponseWriter, req *http.Request) {
	reqPath := req.URL.Path
	subPaths := strings.Split(reqPath, "/")
	if len(subPaths) != 3 {
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	gatewayUID := subPaths[2]

	lock.RLock()
	_, ok := activeGateways[gatewayUID]
	lock.RUnlock()

	if !ok {
		resp.WriteHeader(http.StatusServiceUnavailable)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}
