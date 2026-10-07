package grpcdatasource

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// loadBenchCase is one query scenario for the ds.Load benchmarks.
type loadBenchCase struct {
	name       string
	query      string
	vars       string
	federation plan.FederationFieldConfigurations
}

func entityRepresentations(n int) string {
	var builder strings.Builder
	builder.Grow(n * 40) // approximate size of the string
	builder.WriteString(`{"variables":{"representations":[`)

	for i := range n {

		if i > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder, `{"__typename":"Product","id":"%d"}`, i+1)
	}
	builder.WriteString(`]}}`)
	return builder.String()
}

var loadBenchCases = []loadBenchCase{
	{
		name:  "SimpleList",
		query: `query { users { id name } }`,
		vars:  `{}`,
	},
	{
		name:  "NestedInputObject",
		query: `query($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`,
		vars:  `{"variables":{"filter":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}}`,
	},
	{
		name:  "FieldArgumentsAliases",
		query: `query($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`,
		vars:  `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`,
	},
	{
		name:  "UnionList",
		query: `query($input: SearchInput!) { search(input: $input) { __typename ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }`,
		vars:  `{"variables":{"input":{"query":"test","limit":6}}}`,
	},
	{
		name:  "NestedLists",
		query: `query { author { id name email writtenPosts { id title content } favoriteCategories { id name kind } relatedAuthors { id name } productReviews { id name price } authorGroups { id name } categoryPreferences { id name kind } projectTeams { id name } } }`,
		vars:  `{}`,
	},
	{
		name:  "ListInputScalarLists",
		query: `query($filters: [AuthorFilter!]) { bulkSearchAuthors(filters: $filters) { id name skills languages teamsByProject favoriteCategories { id name kind } categoryPreferences { id name kind } } }`,
		vars:  `{"variables":{"filters":[{"name":"TestAuthor","hasTeams":true,"skillCount":4},{"hasTeams":false,"skillCount":2}]}}`,
	},
	{
		name:  "FieldResolver",
		query: `query($filters: ProductCountFilter) { categories { id name kind productCount(filters: $filters) } }`,
		vars:  `{"variables":{"filters":{"minPrice":100}}}`,
	},
	{
		name:       "Entities_4",
		query:      `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name price } } }`,
		vars:       entityRepresentations(4),
		federation: plan.FederationFieldConfigurations{{TypeName: "Product", SelectionSet: "id"}},
	},
	{
		name:       "Entities_100",
		query:      `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name price } } }`,
		vars:       entityRepresentations(100),
		federation: plan.FederationFieldConfigurations{{TypeName: "Product", SelectionSet: "id"}},
	},
}

// newLoadBenchDataSource creates the data source and the request input for one case.
// It sends one request before the timed loop. The benchmark fails if that request returns errors.
func newLoadBenchDataSource(b *testing.B, conn grpc.ClientConnInterface, tc loadBenchCase) (*DataSource, []byte, int) {
	b.Helper()

	schemaDoc := grpctest.MustGraphQLSchema(b)
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(b), testMapping())
	require.NoError(b, err)

	queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
	if report.HasErrors() {
		b.Fatalf("failed to parse query: %s", report.Error())
	}

	ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
		Operation:         &queryDoc,
		Definition:        &schemaDoc,
		SubgraphName:      "Products",
		Compiler:          compiler,
		Mapping:           testMapping(),
		FederationConfigs: tc.federation,
	})
	require.NoError(b, err)

	input := fmt.Appendf(nil, `{"query":%q,"body":%s}`, tc.query, tc.vars)

	out, err := ds.Load(context.Background(), nil, input)
	require.NoError(b, err)
	require.NotContains(b, string(out), `"errors"`, "response has errors: %s", out)

	return ds, input, len(out)
}

// Benchmark_DataSource_Load_Scenarios measures one full ds.Load call per iteration.
// The call builds the request, sends it over an in-memory gRPC connection and builds the JSON response.
func Benchmark_DataSource_Load_Scenarios(b *testing.B) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	for _, tc := range loadBenchCases {
		b.Run(tc.name, func(b *testing.B) {
			ds, input, size := newLoadBenchDataSource(b, conn, tc)

			b.SetBytes(int64(size))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := ds.Load(context.Background(), nil, input); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark_DataSource_Load_Scenarios_Parallel sends the same calls from many goroutines.
// It shows the behavior under contention, for example allocator and GC load.
func Benchmark_DataSource_Load_Scenarios_Parallel(b *testing.B) {
	conn, cleanup := setupTestGRPCServer(b)
	b.Cleanup(cleanup)

	for _, tc := range loadBenchCases {
		b.Run(tc.name, func(b *testing.B) {
			ds, input, _ := newLoadBenchDataSource(b, conn, tc)

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := ds.Load(context.Background(), nil, input); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}
