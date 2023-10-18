// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
package cmd

import (
	"fmt"
	"net"
	"os"

	"github.com/go-logr/zapr"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_zap "github.com/grpc-ecosystem/go-grpc-middleware/logging/zap"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_ctxtags "github.com/grpc-ecosystem/go-grpc-middleware/tags"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	current "github.com/Azure/kube-egress-gateway/api/v1alpha1"
	"github.com/Azure/kube-egress-gateway/controllers/cnimanager"
	cniconf "github.com/Azure/kube-egress-gateway/pkg/cni/conf"
	cniprotocol "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/logger"
)

const (
	protocol = "unix"
	sockAddr = consts.CNISocketPath
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "cni daemon service",
	Long:  `A daemon service that serves cni requests from cni plugin`,
	Run:   ServiceLauncher,
}

var (
	confFileName   string
	exceptionCidrs string
)

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	serveCmd.Flags().StringVar(&exceptionCidrs, "exception-cidrs", "", "Cidrs that should bypass egress gateway separated with ',', e.g. intra-cluster traffic")
	serveCmd.Flags().StringVar(&confFileName, "cni-conf-file", "01-egressgateway.conflist", "Name of the new cni configuration file")
}

func ServiceLauncher(cmd *cobra.Command, args []string) {
	ctx := signals.SetupSignalHandler()
	zapLog, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}
	logger.SetDefaultLogger(zapr.NewLogger(zapLog))
	logger := logger.GetLogger()

	cniConfMgr, err := cniconf.NewCNIConfManager(consts.CNIConfDir, confFileName, exceptionCidrs)
	if err != nil {
		logger.Error(err, "failed to create cni config manager")
		os.Exit(1)
	}
	go func() {
		if err := cniConfMgr.Start(ctx); err != nil {
			logger.Error(err, "failed to start cni config manager monitoring")
			os.Exit(1)
		}
	}()

	apischeme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(apischeme))
	utilruntime.Must(current.AddToScheme(apischeme))
	k8sCluster, err := cluster.New(config.GetConfigOrDie(), func(options *cluster.Options) {
		options.Scheme = apischeme
		options.Logger = logger
	})
	if err != nil {
		logger.Error(err, "failed to create k8s cluster object")
		os.Exit(1)
	}
	go func() {
		if err := k8sCluster.Start(ctx); err != nil {
			logger.Error(err, "failed to start k8s client cache")
			os.Exit(1)
		}
	}()

	k8sClient := k8sCluster.GetClient()
	nicSvc := cnimanager.NewNicService(k8sClient)

	server := grpc.NewServer(
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(
			grpc_ctxtags.StreamServerInterceptor(),
			grpc_zap.StreamServerInterceptor(zapLog),
			otelgrpc.StreamServerInterceptor(),
			grpc_prometheus.StreamServerInterceptor,
			grpc_recovery.StreamServerInterceptor(),
		)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(
			grpc_ctxtags.UnaryServerInterceptor(),
			grpc_zap.UnaryServerInterceptor(zapLog),
			otelgrpc.UnaryServerInterceptor(),
			grpc_prometheus.UnaryServerInterceptor,
			grpc_recovery.UnaryServerInterceptor(),
		)),
	)

	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	healthgrpc.RegisterHealthServer(server, healthServer)

	cniprotocol.RegisterNicServiceServer(server, nicSvc)
	var listener net.Listener
	listener, err = net.Listen(protocol, sockAddr)
	if err != nil {
		logger.Error(err, "failed to listen")
		os.Exit(1)
	}

	go func() {
		<-ctx.Done()
		logger.Error(ctx.Err(), "os signal received, shutting down")
		server.GracefulStop()
	}()
	err = server.Serve(listener)
	if err != nil {
		logger.Error(err, "failed to serve")
	}
	logger.Info("server shutdown")
}
