package grpcdatasource

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// TestV2_Robustness_FederationShapes drives V2 against representative shapes
// from the federation test suite. For each shape we:
//   1. Build both V1 and V2 datasources
//   2. Check whether V2 compiled natively
//   3. Run the query through both
//   4. Assert byte-identical output
//
// This surfaces the real fallbackReasons backlog: each declined shape is
// printed so the expansion plan is data-driven.
func TestV2_Robustness_FederationShapes(t *testing.T) {
	cases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
	}{
		{
			name:  "entities_product_storage_mixed",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name } ...on Storage { id name } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Product","id":"1"},
				{"__typename":"Storage","id":"3"},
				{"__typename":"Product","id":"2"},
				{"__typename":"Storage","id":"4"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{TypeName: "Product", SelectionSet: "id"},
				{TypeName: "Storage", SelectionSet: "id"},
			},
		},
		{
			name:  "entities_product_with_price",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name price } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Product","id":"1"},
				{"__typename":"Product","id":"2"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{TypeName: "Product", SelectionSet: "id"},
			},
		},
		{
			name:              "categories_simple",
			query:             `query { categories { id name } }`,
			vars:              `{"variables":{}}`,
			federationConfigs: nil,
		},
		{
			name:              "users_simple",
			query:             `query { users { id name } }`,
			vars:              `{"variables":{}}`,
			federationConfigs: nil,
		},
		{
			name:              "categories_with_metrics",
			query:             `query CategoriesWithNullableTypes($nullType: String, $valueType: String) { categories { nullMetrics: categoryMetrics(metricType: $nullType) { id metricType value } valueMetrics: categoryMetrics(metricType: $valueType) { id metricType value } } }`,
			vars:              `{"variables":{"nullType":"unavailable","valueType":"popularity_score"}}`,
			federationConfigs: nil,
		},
	}

	type report struct {
		name     string
		native   bool
		reasons  []string
		parityOK bool
	}
	var results []report

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, gjson.Valid(tc.vars))

			connA, cleanupA := setupTestGRPCServer(t)
			defer cleanupA()
			connB, cleanupB := setupTestGRPCServer(t)
			defer cleanupB()

			schemaDoc := grpctest.MustGraphQLSchema(t)
			queryDoc, parseReport := astparser.ParseGraphqlDocumentString(tc.query)
			require.False(t, parseReport.HasErrors(), "parse: %s", parseReport.Error())

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			require.NoError(t, err)

			cfg := DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Compiler:          compiler,
				Mapping:           testMapping(),
				FederationConfigs: tc.federationConfigs,
			}

			v1, err := NewDataSource(connA, cfg)
			require.NoError(t, err)
			v2, err := NewDataSourceV2(connB, cfg)
			require.NoError(t, err)

			rpt := report{
				name:    tc.name,
				native:  v2.program.nativeOperation,
				reasons: v2.program.fallbackReasons,
			}

			input := []byte(fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars))

			v1Value, v1Cleanup, err := v1.Load(context.Background(), nil, input)
			require.NoError(t, err, "V1 Load failed")
			v2Value, v2Cleanup, err := v2.Load(context.Background(), nil, input)
			require.NoError(t, err, "V2 Load failed")

			v1Bytes := v1Value.MarshalTo(nil)
			v2Bytes := v2Value.MarshalTo(nil)

			if string(v1Bytes) == string(v2Bytes) {
				rpt.parityOK = true
			} else {
				t.Logf("V1 output: %s", string(v1Bytes))
				t.Logf("V2 output: %s", string(v2Bytes))
			}
			require.Equal(t, string(v1Bytes), string(v2Bytes),
				"parity failed for %s (native=%v)", tc.name, rpt.native)

			if v1Cleanup != nil {
				v1Cleanup()
			}
			if v2Cleanup != nil {
				v2Cleanup()
			}

			results = append(results, rpt)
		})
	}

	// Report summary at the end — useful for driving the expansion backlog.
	t.Log("=== V2 coverage summary ===")
	nativeCount := 0
	for _, r := range results {
		status := "FALLBACK"
		if r.native {
			status = "NATIVE"
			nativeCount++
		}
		t.Logf("  %s  %-40s parity=%v", status, r.name, r.parityOK)
		for _, reason := range r.reasons {
			t.Logf("      reason: %s", reason)
		}
	}
	t.Logf("%d/%d shapes native", nativeCount, len(results))
}
