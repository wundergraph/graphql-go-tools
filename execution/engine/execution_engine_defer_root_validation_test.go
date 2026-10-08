package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_DeferRootValidation(t *testing.T) {
	const definition = `
		type Query {
			a: A
		}
		type Mutation {
			createA: A
		}
		type Subscription {
			a: A
		}
		type A {
			id: ID!
		}
	`
	schema, err := graphql.NewSchemaFromString(definition)
	require.NoError(t, err)

	// The round tripper has no response, because validation rejects the operation before planning.
	dataSources := []plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testNetHttpClient(t, roundTripperTestCase{
				expectedHost: "example.com",
				expectedPath: "/",
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"a"}},
					{TypeName: "Mutation", FieldNames: []string{"createA"}},
					{TypeName: "Subscription", FieldNames: []string{"a"}},
				},
				ChildNodes: []plan.TypeField{{TypeName: "A", FieldNames: []string{"id"}}},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch:               &graphql_datasource.FetchConfiguration{URL: "https://example.com/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t, nil, definition),
			}),
		),
	}

	t.Run("[E1] mutation root defer with if false", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				Query: `
					mutation {
						... @defer(if: false) {
							createA { id }
						}
					}`,
			}
		},
		dataSources: dataSources,
		expectedErrorResponse: `{
			"errors": [
				{
					"message": "directive \"@defer\" is not allowed on root fields of mutation operations",
					"locations": [
						{
							"line": 3,
							"column": 11
						}
					],
					"path": ["mutation"]
				}
			]
		}`,
	}, ""))

	t.Run("[E2] mutation root defer with if variable false", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "M",
				Variables:     []byte(`{"d":false}`),
				Query: `
					mutation M($d: Boolean!) {
						... @defer(if: $d) {
							createA { id }
						}
					}`,
			}
		},
		dataSources: dataSources,
		expectedErrorResponse: `{
			"errors": [
				{
					"message": "directive \"@defer\" is not allowed on root fields of mutation operations",
					"locations": [
						{
							"line": 3,
							"column": 11
						}
					],
					"path": ["mutation"]
				}
			]
		}`,
	}, ""))

	t.Run("[E3] nested subscription defer in a fragment before the subscription", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				Query: `
					fragment F on A {
						... @defer { id }
					}
					subscription {
						a { ...F }
					}`,
			}
		},
		dataSources: dataSources,
		expectedErrorResponse: `{
			"errors": [
				{
					"message": "directive \"@defer\" is not allowed on subscription operations",
					"locations": [
						{
							"line": 3,
							"column": 11
						}
					],
					"path": ["A"]
				}
			]
		}`,
	}, ""))
}
