package postprocess

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/staticdatasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// Regression for ROUTER-789 / cosmo#2346: merging the fragment selections must
// preserve nested fields for both concrete types, regardless of fragment order.
func TestMergeFields_ParentTypeConditions(t *testing.T) {
	t.Parallel()

	schema, err := os.ReadFile("testdata/merge_fields_parent_types/schema.graphql")
	require.NoError(t, err)

	cases := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "objects",
			input: `{"product":{"__typename":"%s","category":{"id":"category-id","owner":{"name":"owner-name","aOnly":"a","bOnly":"b"}}}}`,
			want: map[string]string{
				"shared":   `{"data":{"product":{"category":{"id":"category-id","owner":{"name":"owner-name"}}}}}`,
				"ProductA": `{"data":{"product":{"category":{"id":"category-id","owner":{"name":"owner-name","aOnly":"a"}}}}}`,
				"ProductB": `{"data":{"product":{"category":{"id":"category-id","owner":{"name":"owner-name","bOnly":"b"}}}}}`,
				"ProductC": `{"data":{"product":{"category":{"id":"category-id"}}}}`,
			},
		},
		{
			name:  "category_list",
			input: `{"product":{"__typename":"%s","categories":[{"id":"category-id","owner":{"name":"owner-name","aOnly":"a","bOnly":"b"}}]}}`,
			want: map[string]string{
				"shared":   `{"data":{"product":{"categories":[{"id":"category-id","owner":{"name":"owner-name"}}]}}}`,
				"ProductA": `{"data":{"product":{"categories":[{"id":"category-id","owner":{"name":"owner-name","aOnly":"a"}}]}}}`,
				"ProductB": `{"data":{"product":{"categories":[{"id":"category-id","owner":{"name":"owner-name","bOnly":"b"}}]}}}`,
				"ProductC": `{"data":{"product":{"categories":[{"id":"category-id"}]}}}`,
			},
		},
		{
			name:  "owner_list",
			input: `{"product":{"__typename":"%s","category":{"id":"category-id","owners":[{"name":"owner-name","aOnly":"a","bOnly":"b"}]}}}`,
			want: map[string]string{
				"shared":   `{"data":{"product":{"category":{"id":"category-id","owners":[{"name":"owner-name"}]}}}}`,
				"ProductA": `{"data":{"product":{"category":{"id":"category-id","owners":[{"name":"owner-name","aOnly":"a"}]}}}}`,
				"ProductB": `{"data":{"product":{"category":{"id":"category-id","owners":[{"name":"owner-name","bOnly":"b"}]}}}}`,
				"ProductC": `{"data":{"product":{"category":{"id":"category-id"}}}}`,
			},
		},
		{
			name:  "deep_objects",
			input: `{"product":{"__typename":"%s","category":{"id":"category-id","owner":{"profile":{"name":"owner-name","aOnly":"a","bOnly":"b"}}}}}`,
			want: map[string]string{
				"shared":   `{"data":{"product":{"category":{"id":"category-id","owner":{"profile":{"name":"owner-name"}}}}}}`,
				"ProductA": `{"data":{"product":{"category":{"id":"category-id","owner":{"profile":{"name":"owner-name","aOnly":"a"}}}}}}`,
				"ProductB": `{"data":{"product":{"category":{"id":"category-id","owner":{"profile":{"name":"owner-name","bOnly":"b"}}}}}}`,
				"ProductC": `{"data":{"product":{"category":{"id":"category-id"}}}}`,
			},
		},
	}
	selections := []struct {
		name     string
		productA string
		productB string
	}{
		{name: "shared_fields", productA: "name", productB: "name"},
		{name: "distinct_fields", productA: "name aOnly", productB: "name bOnly"},
	}
	orders := []struct {
		name  string
		types [2]string
	}{
		{name: "ProductB_first", types: [2]string{"ProductB", "ProductA"}},
		{name: "ProductA_first", types: [2]string{"ProductA", "ProductB"}},
	}
	runtimeTypes := []string{"ProductA", "ProductB", "ProductC"}

	for _, tc := range cases {
		queryTemplate, err := os.ReadFile("testdata/merge_fields_parent_types/" + tc.name + ".graphql.tmpl")
		require.NoError(t, err)
		for _, selection := range selections {
			for _, order := range orders {
				for _, runtimeType := range runtimeTypes {
					name := fmt.Sprintf("%s/%s/%s/%s", tc.name, selection.name, order.name, runtimeType)
					t.Run(name, func(t *testing.T) {
						t.Parallel()

						fields := map[string]string{"ProductA": selection.productA, "ProductB": selection.productB}
						query := fmt.Sprintf(string(queryTemplate), order.types[0], fields[order.types[0]], order.types[1], fields[order.types[1]])
						response := parentTypeResponsePlan(t, string(schema), query)
						(&mergeFields{}).Process(response)

						// ProductC never selects owner. A and B must include only
						// the leaves requested by their own fragment.
						expected := runtimeType
						if selection.name == "shared_fields" && runtimeType != "ProductC" {
							expected = "shared"
						}
						resolvable := resolve.NewResolvable(nil, resolve.ResolvableOptions{})
						input := fmt.Sprintf(tc.input, runtimeType)
						require.NoError(t, resolvable.Init(&resolve.Context{}, []byte(input), ast.OperationTypeQuery))
						var out bytes.Buffer
						require.NoError(t, resolvable.Resolve(context.Background(), response, nil, &out))
						require.JSONEq(t, tc.want[expected], out.String())
					})
				}
			}
		}
	}
}

// Build the response tree from GraphQL so each case can show the actual query
// rather than manually recreating the planner's nested resolve.Object nodes.
func parentTypeResponsePlan(t *testing.T, schema, query string) *resolve.Object {
	t.Helper()

	definition, report := astparser.ParseGraphqlDocumentString(schema)
	require.False(t, report.HasErrors(), report.Error())
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definition))
	operation, report := astparser.ParseGraphqlDocumentString(query)
	require.False(t, report.HasErrors(), report.Error())
	astnormalization.NewNormalizer(true, true).NormalizeOperation(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())
	astvalidation.DefaultOperationValidator().Validate(&operation, &definition, &report)
	require.False(t, report.HasErrors(), report.Error())

	// A single static source owns every field. These tests exercise response
	// merging/rendering directly, so no upstream request is needed.
	dataSource, err := plan.NewDataSourceConfiguration(
		"products", &staticdatasource.Factory[staticdatasource.Configuration]{},
		&plan.DataSourceMetadata{
			RootNodes: []plan.TypeField{{TypeName: "Query", FieldNames: []string{"product"}}},
			ChildNodes: []plan.TypeField{
				{TypeName: "Base", FieldNames: []string{"category", "categories"}},
				{TypeName: "ProductA", FieldNames: []string{"category", "categories"}},
				{TypeName: "ProductB", FieldNames: []string{"category", "categories"}},
				{TypeName: "ProductC", FieldNames: []string{"category", "categories"}},
				{TypeName: "Category", FieldNames: []string{"id", "owner", "owners"}},
				{TypeName: "Owner", FieldNames: []string{"name", "aOnly", "bOnly", "profile"}},
				{TypeName: "Profile", FieldNames: []string{"name", "aOnly", "bOnly"}},
			},
		}, staticdatasource.Configuration{},
	)
	require.NoError(t, err)
	planner, err := plan.NewPlanner(plan.Configuration{DataSources: []plan.DataSource{dataSource}})
	require.NoError(t, err)
	responsePlan := planner.Plan(&operation, &definition, "", &report)
	require.False(t, report.HasErrors(), report.Error())
	require.IsType(t, &plan.SynchronousResponsePlan{}, responsePlan)
	return responsePlan.(*plan.SynchronousResponsePlan).Response.Data
}
