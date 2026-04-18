package grpcdatasource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// TestDataSourceV2_Parity runs the same input through V1 and V2 and asserts
// the emitted JSON is byte-identical. This is the correctness gate for any
// V2 coverage expansion — a V2 that differs from V1 is wrong by definition,
// regardless of whether it's "fixing" a V1 bug.
func TestDataSourceV2_Parity(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		variables string
	}{
		{
			name:      "simple_users",
			query:     `query { users { id name } }`,
			variables: `{"variables":{}}`,
		},
		{
			name:      "with_field_arguments",
			query:     `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`,
			variables: `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connA, cleanupA := setupTestGRPCServer(t)
			defer cleanupA()
			connB, cleanupB := setupTestGRPCServer(t)
			defer cleanupB()

			schemaDoc := grpctest.MustGraphQLSchema(t)
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, report.HasErrors(), "parse: %s", report.Error())

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			require.NoError(t, err)

			v1, err := NewDataSource(connA, DataSourceConfig{
				Operation: &queryDoc, Definition: &schemaDoc,
				SubgraphName: "Products", Compiler: compiler, Mapping: testMapping(),
			})
			require.NoError(t, err)

			v2, err := NewDataSourceV2(connB, DataSourceConfig{
				Operation: &queryDoc, Definition: &schemaDoc,
				SubgraphName: "Products", Compiler: compiler, Mapping: testMapping(),
			})
			require.NoError(t, err)

			input := []byte(`{"query":"` + tc.query + `","body":` + tc.variables + `}`)

			v1Value, v1Cleanup, err := v1.Load(context.Background(), nil, input)
			require.NoError(t, err)
			v2Value, v2Cleanup, err := v2.Load(context.Background(), nil, input)
			require.NoError(t, err)

			require.Equal(t, string(v1Value.MarshalTo(nil)), string(v2Value.MarshalTo(nil)),
				"V2 output must match V1 for %s — native=%v", tc.name, v2.program.nativeOperation)

			if v1Cleanup != nil {
				v1Cleanup()
			}
			if v2Cleanup != nil {
				v2Cleanup()
			}
		})
	}
}

// Fallback-reason tracking is exercised organically: any query whose plan
// the V2 compiler declines will set `program.nativeOperation = false` and
// route through V1. Further coverage tests can be added per shape as the V2
// supported-subset expands.
