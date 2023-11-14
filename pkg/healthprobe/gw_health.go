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

type LBProbeServer struct {
	lock           sync.RWMutex
	activeGateways map[string]bool
	listenPort     int
}

func NewLBProbeServer(listenPort int) *LBProbeServer {
	return &LBProbeServer{
		activeGateways: make(map[string]bool),
		listenPort:     listenPort,
	}
}

func (svr *LBProbeServer) Start(ctx context.Context) error {
	log := log.FromContext(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc(consts.GatewayHealthProbeEndpoint, svr.serveHTTP)

	httpServer := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(svr.listenPort)),
		Handler:           mux,
		MaxHeaderBytes:    1 << 20,
		IdleTimeout:       90 * time.Second, // matches http.DefaultTransport keep-alive timeout
		ReadHeaderTimeout: 32 * time.Second,
	}

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

func (svr *LBProbeServer) AddGateway(gatewayUID string) error {
	svr.lock.Lock()
	defer svr.lock.Unlock()

	svr.activeGateways[gatewayUID] = true
	return nil
}

func (svr *LBProbeServer) RemoveGateway(gatewayUID string) error {
	svr.lock.Lock()
	defer svr.lock.Unlock()

	delete(svr.activeGateways, gatewayUID)
	return nil
}

func (svr *LBProbeServer) serveHTTP(resp http.ResponseWriter, req *http.Request) {
	reqPath := req.URL.Path
	subPaths := strings.Split(reqPath, "/")
	if len(subPaths) != 3 {
		resp.WriteHeader(http.StatusBadRequest)
		return
	}
	gatewayUID := subPaths[2]

	svr.lock.RLock()
	_, ok := svr.activeGateways[gatewayUID]
	svr.lock.RUnlock()

	if !ok {
		resp.WriteHeader(http.StatusServiceUnavailable)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}
