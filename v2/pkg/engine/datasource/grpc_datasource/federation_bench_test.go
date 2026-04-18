package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// federationBenchSetup builds a DataSource that resolves an `_entities` query across
// two types (Product, Storage) with N representations — exactly the kind of
// fan-out workload Cosmo Router issues. Each representation triggers its own gRPC
// invocation, so this benchmark's per-op cost scales linearly with N.
func federationBenchSetup(b *testing.B) (ds *DataSource, input []byte) {
	b.Helper()

	query := `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name price } ...on Storage { id name location } } }`
	vars := `{"variables":{"representations":[
		{"__typename":"Product","id":"1"},
		{"__typename":"Storage","id":"3"},
		{"__typename":"Product","id":"2"},
		{"__typename":"Storage","id":"4"},
		{"__typename":"Product","id":"5"},
		{"__typename":"Storage","id":"6"},
		{"__typename":"Product","id":"7"},
		{"__typename":"Storage","id":"8"}
	]}}`

	schemaDoc := grpctest.MustGraphQLSchema(b)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(b, report.HasErrors(), "parse: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	ds, err = NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      testMapping(),
		Compiler:     compiler,
		FederationConfigs: plan.FederationFieldConfigurations{
			{TypeName: "Product", SelectionSet: "id"},
			{TypeName: "Storage", SelectionSet: "id"},
		},
	})
	require.NoError(b, err)

	input = []byte(`{"query":"` + query + `","body":` + vars + `}`)
	return ds, input
}

// Benchmark_DataSource_Load_Federation_8Entities — 8-entity entities query, dynamicpb path.
func Benchmark_DataSource_Load_Federation_8Entities(b *testing.B) {
	ds, input := federationBenchSetup(b)
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
