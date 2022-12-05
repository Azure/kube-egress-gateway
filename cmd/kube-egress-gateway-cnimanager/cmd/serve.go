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
package cmd

import (
	"fmt"
	"net"

	"github.com/Azure/kube-egress-gateway/pkg/cnimanager/nicservice"
	cniprotocol "github.com/Azure/kube-egress-gateway/pkg/cniprotocol/v1"
	"github.com/Azure/kube-egress-gateway/pkg/consts"
	"github.com/Azure/kube-egress-gateway/pkg/logger"
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
)

const (
	protocol = "unix"
	sockAddr = consts.CNI_SOCKET_PATH
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "cni daemon service",
	Long:  `A daemon service that serves cni requests from cni plugin`,
	Run:   ServiceLauncher,
}

func init() {
	rootCmd.AddCommand(serveCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// serveCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// serveCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func ServiceLauncher(cmd *cobra.Command, args []string) {
	zapLog, err := zap.NewProduction()
	if err != nil {
		panic(fmt.Sprintf("who watches the watchmen (%v)?", err))
	}
	logger.SetDefaultLogger(zapr.NewLogger(zapLog))
	logger := logger.GetLogger()
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

	cniprotocol.RegisterNicServiceServer(server, &nicservice.NicService{})
	listener, err := net.Listen(protocol, sockAddr)
	if err != nil {
		logger.Error(err, "failed to listen")
	}
	err = server.Serve(listener)
	if err != nil {
		logger.Error(err, "failed to serve")
	}
}
