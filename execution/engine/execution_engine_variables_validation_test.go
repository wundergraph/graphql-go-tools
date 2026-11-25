package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_VariablesValidation(t *testing.T) {
	definition := `
		type Query {
			product: Product!
		}
		type Product {
			id: ID!
			name: String!
		}
	`
	schema, err := graphql.NewSchemaFromString(definition)
	require.NoError(t, err)

	// The round tripper has no response, because variables validation rejects the operation before planning.
	dataSources := []plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testNetHttpClient(t, roundTripperTestCase{
				expectedHost: "first",
				expectedPath: "/",
			})),
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

	t.Run("[E1] request without variables misses a required variable", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "Q",
				Query: `
					query Q($d: Boolean!) {
						product {
							id @skip(if: $d)
							name
						}
					}`,
			}
		},
		dataSources: dataSources,
	}, `Variable "$d" of required type "Boolean!" was not provided.`))

	t.Run("[E2] request with null variables misses a required variable", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "Q",
				Variables:     []byte("null"),
				Query: `
					query Q($d: Boolean!) {
						product {
							id @skip(if: $d)
							name
						}
					}`,
			}
		},
		dataSources: dataSources,
	}, `Variable "$d" of required type "Boolean!" was not provided.`))
}
