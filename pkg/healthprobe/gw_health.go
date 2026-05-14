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

const (
	// DefaultLBProbeDrainDelaySeconds is the default time to wait after marking unhealthy before
	// shutting down the HTTP server. This gives the Azure LB time to detect failed health
	// probes and stop routing traffic to this node.
	// Default Azure LB probe: 5s interval, 2 consecutive failures = 10s to mark down.
	// We use 30s to provide ample margin for LB probe detection and connection draining.
	DefaultLBProbeDrainDelaySeconds = 30 * time.Second
)

type LBProbeServer struct {
	lock           sync.RWMutex
	activeGateways map[string]bool
	listenPort     int
	drainDelay     time.Duration
	shuttingDown   bool
}

func NewLBProbeServer(listenPort int, drainDelay time.Duration) *LBProbeServer {
	if drainDelay <= 0 {
		drainDelay = DefaultLBProbeDrainDelaySeconds
	}
	return &LBProbeServer{
		activeGateways: make(map[string]bool),
		listenPort:     listenPort,
		drainDelay:     drainDelay,
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

	// Shutdown gracefully when context is cancelled:
	// 1. Mark as shutting down so health probes return 503
	// 2. Wait for the LB to detect unhealthy status (2x probe interval)
	// 3. Gracefully shutdown the HTTP server
	<-ctx.Done()
	log.Info("Marking gateway lb health probe server as shutting down")
	svr.lock.Lock()
	svr.shuttingDown = true
	svr.lock.Unlock()

	// Wait for Azure LB to detect unhealthy probes.
	log.Info("Waiting for LB probes to detect shutdown", "delay", svr.drainDelay)
	time.Sleep(svr.drainDelay)

	log.Info("Stopping gateway lb health probe server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "failed to gracefully shutdown gateway lb health probe server")
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

func (svr *LBProbeServer) GetGateways() []string {
	var res []string
	svr.lock.RLock()
	defer svr.lock.RUnlock()

	for gatewayUID := range svr.activeGateways {
		res = append(res, gatewayUID)
	}
	return res
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
	shutting := svr.shuttingDown
	_, ok := svr.activeGateways[gatewayUID]
	svr.lock.RUnlock()

	if shutting || !ok {
		resp.WriteHeader(http.StatusServiceUnavailable)
	} else {
		resp.WriteHeader(http.StatusOK)
	}
}
