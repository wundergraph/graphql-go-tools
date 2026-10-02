package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_DeferLabels(t *testing.T) {
	definition := `type Query { user: User! } type User { id: ID! name: String! title: String! }`

	schema, err := graphql.NewSchemaFromString(definition)
	require.NoError(t, err)

	// The round tripper has no response, because validation rejects the operation before planning.
	dataSources := []plan.DataSource{
		mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testNetHttpClient(t, roundTripperTestCase{
				expectedHost: "first",
				expectedPath: "/",
			})),
			&plan.DataSourceMetadata{
				RootNodes:  []plan.TypeField{{TypeName: "Query", FieldNames: []string{"user"}}},
				ChildNodes: []plan.TypeField{{TypeName: "User", FieldNames: []string{"id", "name", "title"}}},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch:               &graphql_datasource.FetchConfiguration{URL: "https://first/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t, nil, definition),
			}),
		),
	}

	t.Run("[E1] label with variable if reserves its value", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "Q",
				Variables:     []byte(`{"enabled":true}`),
				Query: `query Q($enabled: Boolean!) {
  user {
    ... @defer(label: "details", if: $enabled) { name }
    ... @defer(label: "details") { title }
  }
}`,
			}
		},
		dataSources:           dataSources,
		expectedErrorResponse: `{"errors":[{"message":"directive \"@defer\" label \"details\" must be unique, but was already used on \"@defer\" directive","locations":[{"line":3,"column":9},{"line":4,"column":9}],"path":["query","user"]}]}`,
	}, ""))

	t.Run("[E2] label in a fragment defined before the operation", runWithAndCompareError(ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{
				OperationName: "Q",
				Query: `fragment UserDetails on User {
  ... @defer(label: "details") { name }
}
query Q {
  user {
    ...UserDetails
    ... @defer(label: "details") { title }
  }
}`,
			}
		},
		dataSources:           dataSources,
		expectedErrorResponse: `{"errors":[{"message":"directive \"@defer\" label \"details\" must be unique, but was already used on \"@defer\" directive","locations":[{"line":2,"column":7},{"line":7,"column":9}],"path":["query","user"]}]}`,
	}, ""))
}
