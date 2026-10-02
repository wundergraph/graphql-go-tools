package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_DeferIfArgument(t *testing.T) {
	definition := `
		type Query { product: Product! }
		type Product { id: ID! name: String! }
	`
	schema, err := graphql.NewSchemaFromString(definition)
	require.NoError(t, err)

	dataSources := []plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t,
				testConditionalNetHttpClient(t, conditionalTestCase{
					expectedHost: "first",
					expectedPath: "/",
					responses: map[string]sendResponse{
						`{"query":"{product {id}}"}`: {
							statusCode: 200,
							body:       `{"data":{"product":{"id":"1"}}}`,
						},
						`{"query":"{product {name}}"}`: {
							statusCode: 200,
							body:       `{"data":{"product":{"name":"Table"}}}`,
						},
						`{"query":"{product {id name}}"}`: {
							statusCode: 200,
							body:       `{"data":{"product":{"id":"1","name":"Table"}}}`,
						},
					},
				}),
			),
			&plan.DataSourceMetadata{
				RootNodes:  []plan.TypeField{{TypeName: "Query", FieldNames: []string{"product"}}},
				ChildNodes: []plan.TypeField{{TypeName: "Product", FieldNames: []string{"id", "name"}}},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch:               &graphql_datasource.FetchConfiguration{URL: "https://first/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t, nil, definition),
			}),
		),
	}

	operation := func(query, variables string) func(t *testing.T) graphql.Request {
		return func(t *testing.T) graphql.Request {
			return graphql.Request{OperationName: "Q", Query: query, Variables: []byte(variables)}
		}
	}

	t.Run("[E1] absent variable defers the fragment", runWithoutError(ExecutionEngineTestCase{
		schema:      schema,
		operation:   operation(`query Q($defer: Boolean) { product { id ... @defer(if: $defer) { name } } }`, `{}`),
		dataSources: dataSources,
		expectedResponse: `{"data":{"product":{"id":"1"}},"pending":[{"id":"1","path":["product"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Table"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
	}, withStreamingResponse()))

	t.Run("[E2] absent variable with default false does not defer the fragment", runWithoutError(ExecutionEngineTestCase{
		schema:           schema,
		operation:        operation(`query Q($defer: Boolean = false) { product { id ... @defer(if: $defer) { name } } }`, `{}`),
		dataSources:      dataSources,
		expectedResponse: `{"data":{"product":{"id":"1","name":"Table"}}}`,
	}))

	t.Run("[E3] explicit null is a request error", runWithAndCompareError(ExecutionEngineTestCase{
		schema:                schema,
		operation:             operation(`query Q($defer: Boolean) { product { id ... @defer(if: $defer) { name } } }`, `{"defer":null}`),
		dataSources:           dataSources,
		expectedErrorResponse: `{"errors":[{"message":"Argument \"if\" of non-null type \"Boolean!\" must not be null.","locations":[{"line":1,"column":56}],"path":["query","product"]}]}`,
	}, `Argument "if" of non-null type "Boolean!" must not be null., locations: [{Line:1 Column:56}], path: [query,product]`))

	t.Run("[E4] request without variables misses a required variable", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{OperationName: "Q", Query: `query Q($d: Boolean!) { product { id ... @defer(if: $d) { name } } }`}
		},
		dataSources: dataSources,
	}, `Variable "$d" of required type "Boolean!" was not provided.`))
}
