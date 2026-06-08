package grpcdatasource

import (
	"context"
	"errors"

	"google.golang.org/grpc"
)

// RPCTransport abstracts the transport protocol for RPC calls.
// Both gRPC and Connect protocol implement this interface.
type RPCTransport interface {
	Invoke(ctx context.Context, methodFullName string, input, output any) error
}

// grpcTransport wraps grpc.ClientConnInterface to implement RPCTransport.
type grpcTransport struct {
	cc grpc.ClientConnInterface
}

// NewGRPCTransport creates an RPCTransport that delegates to a gRPC ClientConnInterface.
func NewGRPCTransport(cc grpc.ClientConnInterface, opts ...grpc.CallOption) RPCTransport {
	return &grpcTransport{cc: cc}
}

func (t *grpcTransport) Invoke(ctx context.Context, method string, input, output any) error {
	if t.cc == nil {
		return errors.New("grpc transport: nil client connection")
	}
	// grpc.ClientConnInterface.Invoke accepts (ctx, method, args any, reply any, opts ...grpc.CallOption).
	// protoref.Message satisfies the any constraint; variadic opts can be omitted.
	// This wrapper intentionally does not forward grpc.CallOption, as RPCTransport
	// is protocol-agnostic. The existing grpc_datasource code does not use any CallOption at the Invoke site.
	return t.cc.Invoke(ctx, method, input, output, grpc.ForceCodecV2(&connectCodec{}))
}
