// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
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
