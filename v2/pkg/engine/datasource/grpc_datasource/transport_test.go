package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// newTestCompiler builds an RPCCompiler bound to the grpctest fixture.
// It is shared by every transport-level test in this package.
func newTestCompiler(t *testing.T) *RPCCompiler {
	t.Helper()
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)
	return compiler
}

// findMessageDesc resolves a fully-qualified message name from the compiled
// proto document. Used by tests to construct dynamicpb.Message instances
// for transport.Invoke calls without depending on the generated Go types.
func findMessageDesc(t *testing.T, compiler *RPCCompiler, fullName string) protoref.MessageDescriptor {
	t.Helper()
	for _, m := range compiler.doc.Messages {
		if string(m.Desc.FullName()) == fullName {
			return m.Desc
		}
	}
	t.Fatalf("message %q not found in proto document", fullName)
	return nil
}

// TestGRPCTransport_Invoke is a smoke test for the gRPC RPCTransport
// implementation; it goes through the data source's mockInterface (defined
// in grpc_datasource_test.go) so the assertion is just that Invoke returns
// no error for a well-formed request.
func TestGRPCTransport_Invoke(t *testing.T) {
	mi := mockInterface{}
	transport := NewGRPCTransport(mi)

	compiler := newTestCompiler(t)
	reqDesc := findMessageDesc(t, compiler, "productv1.QueryComplexFilterTypeRequest")
	respDesc := findMessageDesc(t, compiler, "productv1.QueryComplexFilterTypeResponse")

	inputMsg := dynamicpb.NewMessage(reqDesc)
	outputMsg := dynamicpb.NewMessage(respDesc)

	err := transport.Invoke(context.Background(), "/productv1.ProductService/QueryComplexFilterType", inputMsg, outputMsg)
	require.NoError(t, err)
}
