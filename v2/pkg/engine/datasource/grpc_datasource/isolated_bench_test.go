package grpcdatasource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	productv1 "github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
)

// isolatedMockConn implements grpc.ClientConnInterface but bypasses the entire gRPC
// transport: http2, streams, codec registration, metadata handling. It expects a
// prebuilt wire-byte response keyed by method name and calls Unmarshal directly
// on the reply message — exercising only our DataSource plumbing + protobuf
// (un)marshal, not gRPC internals.
//
// This isolates allocation accounting: whatever allocs remain on a bench that
// uses isolatedMockConn belong to our code, protobuf, or astjson — never to
// http2 transport, stream plumbing, or the bufconn mock service machinery.
type isolatedMockConn struct {
	// responses maps full method names (e.g. "/productv1.ProductService/QueryUsers")
	// to a prebuilt wire-byte response for that method.
	responses map[string][]byte
}

var _ grpc.ClientConnInterface = (*isolatedMockConn)(nil)

func (m *isolatedMockConn) Invoke(_ context.Context, method string, args any, reply any, _ ...grpc.CallOption) error {
	wire, ok := m.responses[method]
	if !ok {
		return fmt.Errorf("isolatedMockConn: no canned response for method %s", method)
	}

	switch r := reply.(type) {
	case proto.Message:
		// Covers *dynamicpb.Message and any generated proto.Message impl.
		return proto.Unmarshal(wire, r)
	default:
		return fmt.Errorf("isolatedMockConn: unsupported reply type %T", reply)
	}
}

func (m *isolatedMockConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, fmt.Errorf("isolatedMockConn: streaming unsupported")
}

// buildIsolatedConn produces a mock with QueryUsers prebaked.
func buildIsolatedConn(b testing.TB) *isolatedMockConn {
	b.Helper()

	// Produce real wire bytes by marshaling a representative response once. Matches
	// what MockService.QueryUsers would synthesize at runtime.
	resp := &productv1.QueryUsersResponse{
		Users: []*productv1.User{
			{Id: "user-1", Name: "User 1"},
			{Id: "user-2", Name: "User 2"},
			{Id: "user-3", Name: "User 3"},
		},
	}
	wire, err := proto.Marshal(resp)
	require.NoError(b, err)

	return &isolatedMockConn{
		responses: map[string][]byte{
			productv1.ProductService_QueryUsers_FullMethodName: wire,
		},
	}
}

// isolatedBenchSetup wires a DataSource to an isolatedMockConn for a simple
// `query { users { id name } }` workload.
func isolatedBenchSetup(b *testing.B) (*DataSource, []byte) {
	b.Helper()

	query := `query { users { id name } }`
	variables := `{"variables":{}}`

	schemaDoc := grpctest.MustGraphQLSchema(b)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(b, report.HasErrors(), "parse: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	ds, err := NewDataSource(buildIsolatedConn(b), DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(b, err)

	return ds, []byte(`{"query":"` + query + `","body":` + variables + `}`)
}

// Benchmark_DataSource_Load_Isolated isolates datasource-level allocations from
// gRPC transport. This is the "pure our-code" number.
func Benchmark_DataSource_Load_Isolated(b *testing.B) {
	ds, input := isolatedBenchSetup(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v, cleanup, err := ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
		_ = v
		if cleanup != nil {
			cleanup()
		}
	}
}

// sanity: the isolated path must also produce a correct response.
func TestIsolated_Load_ProducesExpectedJSON(t *testing.T) {
	ds, input := func() (*DataSource, []byte) {
		query := `query { users { id name } }`
		variables := `{"variables":{}}`
		schemaDoc := grpctest.MustGraphQLSchema(t)
		queryDoc, report := astparser.ParseGraphqlDocumentString(query)
		require.False(t, report.HasErrors(), "parse: %s", report.Error())
		compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
		require.NoError(t, err)
		mc := buildIsolatedConnT(t)
		ds, err := NewDataSource(mc, DataSourceConfig{
			Operation:    &queryDoc,
			Definition:   &schemaDoc,
			SubgraphName: "Products",
			Compiler:     compiler,
			Mapping:      testMapping(),
		})
		require.NoError(t, err)
		return ds, []byte(`{"query":"` + query + `","body":` + variables + `}`)
	}()

	out, cleanup, err := ds.Load(context.Background(), nil, input)
	require.NoError(t, err)
	require.Equal(t,
		`{"data":{"users":[{"id":"user-1","name":"User 1"},{"id":"user-2","name":"User 2"},{"id":"user-3","name":"User 3"}]}}`,
		string(out.MarshalTo(nil)))
	if cleanup != nil {
		cleanup()
	}
}

func buildIsolatedConnT(t *testing.T) *isolatedMockConn {
	t.Helper()
	resp := &productv1.QueryUsersResponse{
		Users: []*productv1.User{
			{Id: "user-1", Name: "User 1"},
			{Id: "user-2", Name: "User 2"},
			{Id: "user-3", Name: "User 3"},
		},
	}
	wire, err := proto.Marshal(resp)
	require.NoError(t, err)
	return &isolatedMockConn{
		responses: map[string][]byte{
			productv1.ProductService_QueryUsers_FullMethodName: wire,
		},
	}
}
