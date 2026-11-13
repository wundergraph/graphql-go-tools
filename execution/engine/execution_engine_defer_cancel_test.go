package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

// TestExecutionEngine_Execute_DeferCancelledBeforeInitialResult covers initial null
// propagation that removes defer anchors. A response without a surviving defer is an
// ordinary execution result. A response with a surviving defer lists only that defer.
func TestExecutionEngine_Execute_DeferCancelledBeforeInitialResult(t *testing.T) {
	t.Parallel()

	userDataSources := func(t *testing.T, definition string, rootFields []string, responses map[string]sendResponse) []plan.DataSource {
		t.Helper()

		return []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id-1",
				mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
					expectedHost: "first",
					expectedPath: "/",
					responses:    responses,
				})),
				&plan.DataSourceMetadata{
					RootNodes:  []plan.TypeField{{TypeName: "Query", FieldNames: rootFields}},
					ChildNodes: []plan.TypeField{{TypeName: "User", FieldNames: []string{"name", "title"}}},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{URL: "https://first/", Method: "POST"},
					SchemaConfiguration: mustSchemaConfig(t,
						&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: definition},
						definition,
					),
				}),
			),
		}
	}

	t.Run("[E1] null propagation removes the only defer anchor", func(t *testing.T) {
		t.Parallel()
		definition := `
			type User { name: String! title: String! }
			type Query { user: User }
		`
		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ user { name ... @defer { title } } }`}
			},
			dataSources: userDataSources(t, definition, []string{"user"}, map[string]sendResponse{
				`{"query":"{user {name}}"}`: {statusCode: 200, body: `{"data":{"user":{"name":null}}}`},
			}),
			expectedResponse: `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}
`,
		}, withStreamingResponse())(t)
	})

	t.Run("[E2] null propagation removes one defer anchor and one survives", func(t *testing.T) {
		t.Parallel()
		definition := `
			type User { name: String! title: String! }
			type Query { a: User b: User }
		`
		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ a { name ... @defer { title } } b { name ... @defer { title } } }`}
			},
			dataSources: userDataSources(t, definition, []string{"a", "b"}, map[string]sendResponse{
				`{"query":"{a {name} b {name}}"}`: {statusCode: 200, body: `{"data":{"a":{"name":null},"b":{"name":"B"}}}`},
				`{"query":"{b {title}}"}`:         {statusCode: 200, body: `{"data":{"b":{"title":"TB"}}}`},
			}),
			expectedResponse: `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.a.name'.","path":["a","name"]}],"data":{"a":null,"b":{"name":"B"}},"pending":[{"id":"2","path":["b"]}],"hasNext":true}
{"incremental":[{"data":{"title":"TB"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)
	})

	t.Run("[E3] null propagation reaches the root", func(t *testing.T) {
		t.Parallel()
		definition := `
			type User { name: String! title: String! }
			type Query { user: User! }
		`
		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ user { name ... @defer { title } } }`}
			},
			dataSources: userDataSources(t, definition, []string{"user"}, map[string]sendResponse{
				`{"query":"{user {name}}"}`: {statusCode: 200, body: `{"data":{"user":{"name":null}}}`},
			}),
			expectedResponse: `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":null}
`,
		}, withStreamingResponse())(t)
	})
}
