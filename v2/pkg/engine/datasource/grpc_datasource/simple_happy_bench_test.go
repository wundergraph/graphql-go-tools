package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func simpleHappyBenchSetup(b *testing.B) (*DataSource, []byte) {
	b.Helper()

	// Simple 1-call happy-path query: `users { id name }`.
	// MockService's QueryUsers handler produces a deterministic list; no federation,
	// no field resolvers, no nested entities.
	query := `query { users { id name } }`
	variables := `{"variables":{}}`

	schemaDoc := grpctest.MustGraphQLSchema(b)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(b, report.HasErrors(), "parse: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(b, err)

	return ds, []byte(`{"query":"` + query + `","body":` + variables + `}`)
}

// Benchmark_DataSource_Load_SimpleHappy — 1-call happy path through dynamicpb.
func Benchmark_DataSource_Load_SimpleHappy(b *testing.B) {
	ds, input := simpleHappyBenchSetup(b)
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
