package main

// This file is solely used for testing purposes with an actual running gRPC server.
// It implements a mock gRPC server that can be used to test the gRPC datasource
// functionality in the graphql-go-tools library. The server exposes various endpoints
// defined in the product.proto file, allowing for comprehensive testing of GraphQL
// to gRPC mapping, including entity resolution, query operations, interface types,
// enum mapping, and mutation operations. This test server is essential for validating
// that the GraphQL to gRPC integration works correctly in different scenarios.

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/grpc"
)

func loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp interface{}, err error) {
	start := time.Now()
	// Calls the handler to proceed with normal execution of RPC.
	resp, err = handler(ctx, req)
	// After handler completes, log method, duration, error.
	log.Printf(
		"[gRPC] Method=%s Duration=%s Error=%v",
		info.FullMethod,
		time.Since(start),
		err,
	)
	return resp, err
}

func main() {
	l, err := net.Listen("tcp", ":9009")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	s := grpc.NewServer(
		grpc.UnaryInterceptor(loggingInterceptor),
	)
	productv1.RegisterProductServiceServer(s, &grpctest.MockService{})

	log.Printf("Starting gRPC server on port 9009")
	if err := s.Serve(l); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
