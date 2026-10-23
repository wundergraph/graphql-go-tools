package graphql_datasource

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestCompositeNullabilityAliases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, left, right string
		aliases           bool
	}{
		{"Objects", "X!", "Y", true},
		{"NestedLists", "[[X!]!]!", "[[Y]]", true},
		{"MatchingWrappers", "X", "Y", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			schema := fmt.Sprintf(`type Query { entity: Entity } union Entity = A | B
    type A { field: %s } type B { field: %s }
    type X { name: String } type Y { name: String }`, tc.left, tc.right)
			ds := mustDataSourceConfiguration(t, "entities", &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"entity"}}},
				ChildNodes: []plan.TypeField{
					{TypeName: "A", FieldNames: []string{"field"}}, {TypeName: "B", FieldNames: []string{"field"}},
					{TypeName: "X", FieldNames: []string{"name"}}, {TypeName: "Y", FieldNames: []string{"name"}},
				},
			}, mustCustomConfiguration(t, ConfigurationInput{
				Fetch:               &FetchConfiguration{URL: "http://entities"},
				SchemaConfiguration: mustSchema(t, &FederationConfiguration{Enabled: true, ServiceSDL: schema}, schema),
			}))
			result := planSubgraphOperationTest(t, schema,
				`{ entity { ... on A { field { name } } ... on B { field { name } } } }`, "",
				plan.Configuration{DataSources: []plan.DataSource{ds}, DisableResolveFieldPositions: true},
				astvalidation.WithRelaxFieldSelectionMergingNullability())
			require.Len(t, result.Response.RawFetches, 1)
			fetch, ok := result.Response.RawFetches[0].Fetch.(*resolve.SingleFetch)
			require.True(t, ok)
			var request struct{ Body struct{ Query string } }
			require.NoError(t, json.Unmarshal([]byte(fetch.Input), &request))
			if tc.aliases {
				assert.Contains(t, request.Body.Query, "__internal_merge_A_field: field")
				assert.Contains(t, request.Body.Query, "__internal_merge_B_field: field")
			} else {
				assert.NotContains(t, request.Body.Query, "__internal_merge_")
			}
			definition, report := astparser.ParseGraphqlDocumentString(schema)
			require.False(t, report.HasErrors(), report.Error())
			require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))
			operation, report := astparser.ParseGraphqlDocumentString(request.Body.Query)
			require.False(t, report.HasErrors(), report.Error())
			assert.Equal(t, astvalidation.Valid, astvalidation.DefaultOperationValidator().Validate(&operation, &definition, &report), report.Error())
			entity, ok := result.Response.Data.Fields[0].Value.(*resolve.Object)
			require.True(t, ok)
			assert.Len(t, entity.Fields, 2)
			for _, field := range entity.Fields {
				assert.Equal(t, "field", string(field.Name))
				path := "field"
				if tc.aliases {
					require.Len(t, field.OnTypeNames, 1)
					path = "__internal_merge_" + string(field.OnTypeNames[0]) + "_field"
				}
				assert.Equal(t, []string{path}, field.Value.NodePath())
			}
		})
	}
}
