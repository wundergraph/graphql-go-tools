package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// loaderGRPCBenchSetup wires a resolve.Loader to a gRPC DataSource behind an
// isolatedMockConn (no http2, no bufconn). Gives a clean measurement of the
// end-to-end path: resolver-level merge + gRPC datasource + protobuf decode,
// without the gRPC transport noise that dominates the non-isolated benches.
//
// Query: `{ users { id name } }` → one SingleFetch producing
// `{"users":[{id,name}×3]}`. The Loader then renders the response.
func loaderGRPCBenchSetup(b *testing.B) (
	loader *resolve.Loader,
	ctx *resolve.Context,
	resolvable *resolve.Resolvable,
	response *resolve.GraphQLResponse,
) {
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

	// The Loader expects a GraphQLResponse spec describing the fetches to run and
	// the shape of the final response object. For `{ users { id name } }` we need:
	//   - one SingleFetch with our gRPC DataSource + input template containing
	//     the {"query":..., "body": {"variables":{}}} envelope the grpc ds reads
	//   - a Data shape that reads users[].{id,name} from the fetch response
	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	response = &resolve.GraphQLResponse{
		Fetches: resolve.Single(&resolve.SingleFetch{
			InputTemplate: resolve.InputTemplate{
				Segments: []resolve.TemplateSegment{
					{Data: input, SegmentType: resolve.StaticSegmentType},
				},
			},
			FetchConfiguration: resolve.FetchConfiguration{
				DataSource: ds,
				PostProcessing: resolve.PostProcessingConfiguration{
					SelectResponseDataPath: []string{"data"},
				},
			},
		}),
		Data: &resolve.Object{
			Fields: []*resolve.Field{
				{
					Name: []byte("users"),
					Value: &resolve.Array{
						Path: []string{"users"},
						Item: &resolve.Object{
							Fields: []*resolve.Field{
								{Name: []byte("id"), Value: &resolve.String{Path: []string{"id"}}},
								{Name: []byte("name"), Value: &resolve.String{Path: []string{"name"}}},
							},
						},
					},
				},
			},
		},
	}

	ctx = resolve.NewContext(context.Background())
	resolvable = resolve.NewResolvable(nil, resolve.ResolvableOptions{})
	loader = &resolve.Loader{}
	return
}

// Benchmark_Loader_GRPC_End2End drives the full Loader + gRPC datasource chain
// through the isolated mock conn. Measures the post-refactor cost with Value-
// returning datasources and the DeepCopy-or-skip logic added in loader.go.
func Benchmark_Loader_GRPC_End2End(b *testing.B) {
	loader, ctx, resolvable, response := loaderGRPCBenchSetup(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		loader.Free()
		resolvable.Reset()
		if err := resolvable.Init(ctx, nil, ast.OperationTypeQuery); err != nil {
			b.Fatal(err)
		}
		if err := loader.LoadGraphQLResponseData(ctx, response, resolvable); err != nil {
			b.Fatal(err)
		}
	}
}
