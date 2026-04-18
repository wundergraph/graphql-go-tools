package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// Paired V1/V2 benchmarks — every V1 bench gets a V2 twin running the exact
// same workload through the new datasource. The purpose is not to cherry-pick
// a winning shape but to see A/B across the real query suite as the V2
// coverage expands.

func Benchmark_DataSource_V1_Load_SimpleHappy(b *testing.B) {
	runSimpleHappy(b, false)
}

func Benchmark_DataSource_V2_Load_SimpleHappy(b *testing.B) {
	runSimpleHappy(b, true)
}

func Benchmark_DataSource_V1_Load_WithFieldArgs(b *testing.B) {
	runWithFieldArgs(b, false)
}

func Benchmark_DataSource_V2_Load_WithFieldArgs(b *testing.B) {
	runWithFieldArgs(b, true)
}

func runSimpleHappy(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	query := `query { users { id name } }`
	variables := `{"variables":{}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(b, report.HasErrors(), "parse: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	cfg := DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	}
	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)

	if useV2 {
		ds, err := NewDataSourceV2(conn, cfg)
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			v, cl, err := ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
			_ = v
			if cl != nil {
				cl()
			}
		}
		return
	}

	ds, err := NewDataSource(conn, cfg)
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v, cl, err := ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
		_ = v
		if cl != nil {
			cl()
		}
	}
}

func runWithFieldArgs(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	variables := `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	require.False(b, report.HasErrors(), "parse: %s", report.Error())

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	cfg := DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	}
	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)

	if useV2 {
		ds, err := NewDataSourceV2(conn, cfg)
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			v, cl, err := ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
			_ = v
			if cl != nil {
				cl()
			}
		}
		return
	}

	ds, err := NewDataSource(conn, cfg)
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		v, cl, err := ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
		_ = v
		if cl != nil {
			cl()
		}
	}
}
