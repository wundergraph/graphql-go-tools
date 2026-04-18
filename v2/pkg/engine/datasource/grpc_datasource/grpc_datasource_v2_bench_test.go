package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func Benchmark_DataSource_V1_Load(b *testing.B) {
	benchmarkDataSourceVersionLoad(b, false)
}

func Benchmark_DataSource_V2_Load(b *testing.B) {
	benchmarkDataSourceVersionLoad(b, true)
}

func Benchmark_DataSource_V2_LoadValue(b *testing.B) {
	benchmarkDataSourceV2LoadValue(b, benchmarkScenarioLoad)
}

func Benchmark_DataSource_V2_LoadResult(b *testing.B) {
	benchmarkDataSourceV2LoadResult(b, benchmarkScenarioLoad)
}

func Benchmark_DataSource_V1_Load_WithFieldArguments(b *testing.B) {
	benchmarkDataSourceVersionLoadWithFieldArguments(b, false)
}

func Benchmark_DataSource_V2_Load_WithFieldArguments(b *testing.B) {
	benchmarkDataSourceVersionLoadWithFieldArguments(b, true)
}

func Benchmark_DataSource_V2_LoadValue_WithFieldArguments(b *testing.B) {
	benchmarkDataSourceV2LoadValue(b, benchmarkScenarioLoadWithFieldArguments)
}

func Benchmark_DataSource_V2_LoadResult_WithFieldArguments(b *testing.B) {
	benchmarkDataSourceV2LoadResult(b, benchmarkScenarioLoadWithFieldArguments)
}

func Benchmark_DataSource_V1_Load_FederationFanout(b *testing.B) {
	benchmarkDataSourceVersionLoadFederationFanout(b, false)
}

func Benchmark_DataSource_V2_Load_FederationFanout(b *testing.B) {
	benchmarkDataSourceVersionLoadFederationFanout(b, true)
}

func Benchmark_DataSource_V2_LoadValue_FederationFanout(b *testing.B) {
	benchmarkDataSourceV2LoadValue(b, benchmarkScenarioFederationFanout)
}

func Benchmark_DataSource_V2_LoadResult_FederationFanout(b *testing.B) {
	benchmarkDataSourceV2LoadResult(b, benchmarkScenarioFederationFanout)
}

func Benchmark_DataSource_V1_Load_FederationRequiresUnion(b *testing.B) {
	benchmarkDataSourceVersionLoadFederationRequiresUnion(b, false)
}

func Benchmark_DataSource_V2_Load_FederationRequiresUnion(b *testing.B) {
	benchmarkDataSourceVersionLoadFederationRequiresUnion(b, true)
}

func Benchmark_DataSource_V2_LoadValue_FederationRequiresUnion(b *testing.B) {
	benchmarkDataSourceV2LoadValue(b, benchmarkScenarioFederationRequiresUnion)
}

func Benchmark_DataSource_V2_LoadResult_FederationRequiresUnion(b *testing.B) {
	benchmarkDataSourceV2LoadResult(b, benchmarkScenarioFederationRequiresUnion)
}

type benchmarkScenario int

const (
	benchmarkScenarioLoad benchmarkScenario = iota
	benchmarkScenarioLoadWithFieldArguments
	benchmarkScenarioFederationFanout
	benchmarkScenarioFederationRequiresUnion
)

func benchmarkDataSourceVersionLoad(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	if useV2 {
		ds, err := NewDataSourceV2(conn, DataSourceConfig{
			Operation:    &queryDoc,
			Definition:   &schemaDoc,
			SubgraphName: "Products",
			Compiler:     compiler,
			Mapping:      testMapping(),
		})
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err = ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
		}
		return
	}

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
	}
}

func benchmarkDataSourceVersionLoadWithFieldArguments(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	query := `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`
	variables := `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	if useV2 {
		ds, err := NewDataSourceV2(conn, DataSourceConfig{
			Operation:    &queryDoc,
			Definition:   &schemaDoc,
			SubgraphName: "Products",
			Compiler:     compiler,
			Mapping:      testMapping(),
		})
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err = ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
		}
		return
	}

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping:      testMapping(),
	})
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
	}
}

func benchmarkDataSourceV2LoadValue(b *testing.B, scenario benchmarkScenario) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	query, variables, federationConfigs := benchmarkScenarioInput(scenario)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Compiler:          compiler,
		Mapping:           testMapping(),
		FederationConfigs: federationConfigs,
	})
	require.NoError(b, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		value, release, err := ds.LoadValue(context.Background(), nil, input)
		require.NoError(b, err)
		require.NotNil(b, value)
		release()
	}
}

func benchmarkDataSourceV2LoadResult(b *testing.B, scenario benchmarkScenario) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	query, variables, federationConfigs := benchmarkScenarioInput(scenario)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	ds, err := NewDataSourceV2(conn, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Compiler:          compiler,
		Mapping:           testMapping(),
		FederationConfigs: federationConfigs,
	})
	require.NoError(b, err)

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	mergePool := arena.NewArenaPool()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		result, release, err := ds.LoadResult(context.Background(), nil, input)
		require.NoError(b, err)
		require.NotNil(b, result)
		mergeItem := mergePool.Acquire(uint64(i + 1))
		merged, err := result.MergeInto(mergeItem.Arena, nil, resolve.PostProcessingConfiguration{SelectResponseDataPath: []string{"data"}}, nil)
		require.NoError(b, err)
		require.NotNil(b, merged)
		release()
		mergePool.Release(mergeItem)
	}
}

func benchmarkDataSourceVersionLoadFederationFanout(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	query, variables, federationConfigs := benchmarkScenarioInput(benchmarkScenarioFederationFanout)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	if useV2 {
		ds, err := NewDataSourceV2(conn, DataSourceConfig{
			Operation:         &queryDoc,
			Definition:        &schemaDoc,
			SubgraphName:      "Products",
			Compiler:          compiler,
			Mapping:           testMapping(),
			FederationConfigs: federationConfigs,
		})
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err = ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
		}
		return
	}

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Compiler:          compiler,
		Mapping:           testMapping(),
		FederationConfigs: federationConfigs,
	})
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
	}
}

func benchmarkDataSourceVersionLoadFederationRequiresUnion(b *testing.B, useV2 bool) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	schemaDoc := grpctest.MustGraphQLSchema(b)
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	query, variables, federationConfigs := benchmarkScenarioInput(benchmarkScenarioFederationRequiresUnion)
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	input := []byte(`{"query":"` + query + `","body":` + variables + `}`)
	if useV2 {
		ds, err := NewDataSourceV2(conn, DataSourceConfig{
			Operation:         &queryDoc,
			Definition:        &schemaDoc,
			SubgraphName:      "Products",
			Compiler:          compiler,
			Mapping:           testMapping(),
			FederationConfigs: federationConfigs,
		})
		require.NoError(b, err)
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err = ds.Load(context.Background(), nil, input)
			require.NoError(b, err)
		}
		return
	}

	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Compiler:          compiler,
		Mapping:           testMapping(),
		FederationConfigs: federationConfigs,
	})
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err = ds.Load(context.Background(), nil, input)
		require.NoError(b, err)
	}
}

func benchmarkScenarioInput(scenario benchmarkScenario) (query, variables string, federationConfigs plan.FederationFieldConfigurations) {
	switch scenario {
	case benchmarkScenarioLoad:
		return `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
			`{"variables":{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}}`,
			nil
	case benchmarkScenarioLoadWithFieldArguments:
		return `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`,
			`{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`,
			nil
	case benchmarkScenarioFederationFanout:
		return `query($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ...on Product { id name price shippingEstimate(input: $input) } } }`,
			`{"variables":{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"},{"__typename":"Product","id":"3"}],"input":{"destination":"INTERNATIONAL","weight":10.0,"expedited":true}}}`,
			plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			}
	case benchmarkScenarioFederationRequiresUnion:
		return `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } }`,
			`{"variables":{"representations":[{"__typename":"Storage","id":"1","tags":["electronics","gadgets","sale"]},{"__typename":"Storage","id":"2","tags":["books","fiction"]},{"__typename":"Storage","id":"3","tags":[]}],"checkHealth":true}}`,
			plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
			}
	default:
		panic("unsupported benchmark scenario")
	}
}
