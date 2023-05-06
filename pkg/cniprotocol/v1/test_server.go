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
}

func (s *TestServer) NicAdd(ctx context.Context, in *NicAddRequest) (*NicAddResponse, error) {
	s.Received <- in
	return &NicAddResponse{
		EndpointIp:     TEST_SERVER_WIREGUARD_SERVER_IP,
		ListenPort:     TEST_SERVER_WIREGUARD_SERVER_PORT,
		PublicKey:      TEST_SERVER_WIREGUARD_PUBLIC_KEY,
		ExceptionCidrs: s.exceptionCidrs,
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
	s.lis.Close()
}

func (s *TestServer) startServer() {
	err := s.grpcServer.Serve(s.lis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start grpc server: %v", err)
	}
}

func StartTestServer(socket string, exceptionCidrs []string, podAnnotations map[string]string) (s *TestServer, err error) {
	s = &TestServer{
		Received:       make(chan interface{}, 2),
		grpcServer:     grpc.NewServer(),
		podAnnotations: podAnnotations,
		exceptionCidrs: exceptionCidrs,
	}

	s.lis, err = net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on unix socket %s: %v", socket, err)
	}

	RegisterNicServiceServer(s.grpcServer, s)
	go s.startServer()
	return s, nil
}
