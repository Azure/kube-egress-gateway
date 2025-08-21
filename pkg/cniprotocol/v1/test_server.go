// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.

package v1

import (
	"context"
	"fmt"
	"net"
	"os"

	"google.golang.org/grpc"
)

const (
	TEST_SERVER_WIREGUARD_SERVER_IP         = "10.2.0.4"
	TEST_SERVER_WIREGUARD_SERVER_PORT int32 = 6000
	TEST_SERVER_WIREGUARD_PUBLIC_KEY        = "aPxGwq8zERHQ3Q1cOZFdJ+cvJX5Ka4mLN38AyYKYF10="
	TEST_SERVER_WIREGUARD_PRIVATE_KEY       = "GHuMwljFfqd2a7cs6BaUOmHflK23zME8VNvC5B37S3k="
)

type TestServer struct {
	Received chan interface{}
	UnimplementedNicServiceServer

	lis            net.Listener
	grpcServer     *grpc.Server
	podAnnotations map[string]string
	exceptionCidrs []string
	defaultRoute   DefaultRoute
}

func (s *TestServer) NicAdd(ctx context.Context, in *NicAddRequest) (*NicAddResponse, error) {
	s.Received <- in
	return &NicAddResponse{
		EndpointIp:     TEST_SERVER_WIREGUARD_SERVER_IP,
		ListenPort:     TEST_SERVER_WIREGUARD_SERVER_PORT,
		PublicKey:      TEST_SERVER_WIREGUARD_PUBLIC_KEY,
		ExceptionCidrs: s.exceptionCidrs,
		DefaultRoute:   s.defaultRoute,
	}, nil
}

func (s *TestServer) NicDel(ctx context.Context, in *NicDelRequest) (*NicDelResponse, error) {
	s.Received <- in
	return &NicDelResponse{}, nil
}

func (s *TestServer) PodRetrieve(ctx context.Context, in *PodRetrieveRequest) (*PodRetrieveResponse, error) {
	s.Received <- in
	return &PodRetrieveResponse{Annotations: s.podAnnotations}, nil
}

func (s *TestServer) GracefulStop() {
	s.grpcServer.GracefulStop()
	_ = s.lis.Close()
}

func (s *TestServer) startServer() {
	err := s.grpcServer.Serve(s.lis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start grpc server: %v", err)
	}
}

func StartTestServer(addr string, exceptionCidrs []string, podAnnotations map[string]string) (s *TestServer, err error) {
	return StartTestServerWithDefaultRoute(addr, exceptionCidrs, podAnnotations, DefaultRoute_DEFAULT_ROUTE_UNSPECIFIED)
}

func StartTestServerWithDefaultRoute(addr string, exceptionCidrs []string, podAnnotations map[string]string, defaultRoute DefaultRoute) (s *TestServer, err error) {
	s = &TestServer{
		Received:       make(chan interface{}, 2),
		grpcServer:     grpc.NewServer(),
		podAnnotations: podAnnotations,
		exceptionCidrs: exceptionCidrs,
		defaultRoute:   defaultRoute,
	}

	s.lis, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on tcp addr %s: %v", addr, err)
	}

	RegisterNicServiceServer(s.grpcServer, s)
	go s.startServer()
	return s, nil
}
