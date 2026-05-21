package grpcdatasource

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
)

// RPCTransport abstracts the transport protocol for RPC calls.
// Both gRPC and Connect implement this interface.
//
// Invoke dispatches a unary call against the remote service.
//   - methodFullName is the gRPC-style procedure path "/package.Service/Method"
//     (leading slash required). The gRPC transport passes it directly to
//     grpc.ClientConnInterface.Invoke; the Connect transport appends it to
//     the configured base URL.
//   - input must be a *dynamicpb.Message populated by the caller.
//   - output must be a *dynamicpb.Message bound to the expected response
//     descriptor; Invoke populates it on success.
type RPCTransport interface {
	Invoke(ctx context.Context, methodFullName string, input, output protoref.Message) error
}

// grpcTransport wraps grpc.ClientConnInterface to implement RPCTransport.
type grpcTransport struct {
	cc grpc.ClientConnInterface
}

// NewGRPCTransport creates an RPCTransport that delegates to a gRPC ClientConnInterface.
func NewGRPCTransport(cc grpc.ClientConnInterface) RPCTransport {
	return &grpcTransport{cc: cc}
}

func (t *grpcTransport) Invoke(ctx context.Context, method string, input, output protoref.Message) error {
	if t.cc == nil {
		return errors.New("grpc transport: nil client connection")
	}
	// grpc.ClientConnInterface.Invoke accepts (ctx, method, args any, reply any, opts ...grpc.CallOption).
	// protoref.Message satisfies the any constraint; variadic opts can be omitted.
	// This wrapper intentionally does not forward grpc.CallOption, as RPCTransport
	// is protocol-agnostic. The existing grpc_datasource code does not use any CallOption at the Invoke site.
	return t.cc.Invoke(ctx, method, input, output)
}
