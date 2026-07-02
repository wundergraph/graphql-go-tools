package engine

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"

	graphql_datasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
)

// onErrorE2ESchema has a nullable hero whose name is non-null, so a subgraph
// that returns name:null (with an error) triggers a non-null violation that
// each error behavior handles differently.
const onErrorE2ESchema = `
type Query { hero: Hero }
type Hero { id: String name: String! }`

func onErrorE2ECase(t *testing.T, behavior resolve.ErrorBehavior, expected string) ExecutionEngineTestCase {
	t.Helper()
	schema, err := graphql.NewSchemaFromString(onErrorE2ESchema)
	require.NoError(t, err)
	return ExecutionEngineTestCase{
		schema: schema,
		operation: func(t *testing.T) graphql.Request {
			return graphql.Request{Query: `{ hero { id name } }`}
		},
		dataSources: []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id",
				mustFactory(t,
					testNetHttpClient(t, roundTripperTestCase{
						expectedHost:     "example.com",
						expectedPath:     "/",
						expectedBody:     "",
						sendResponseBody: `{"data":{"hero":{"id":"1","name":null}},"errors":[{"message":"boom","path":["hero","name"]}]}`,
						sendStatusCode:   200,
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"hero"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "Hero", FieldNames: []string{"id", "name"}},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://example.com/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(t, nil, onErrorE2ESchema),
				}),
			),
		},
		engineOptions:        []ExecutionOptions{WithErrorBehavior(behavior)},
		expectedJSONResponse: expected,
	}
}

func TestExecutionEngine_OnErrorBehavior(t *testing.T) {
	t.Parallel()

	// The raw subgraph error is wrapped as "Failed to fetch from Subgraph 'id'."
	// by the default subgraph-error propagation mode (orthogonal to onError,
	// RFC §5.6). Under PROPAGATE/NULL the resolver additionally records the
	// non-null violation; HALT trims both down to the single first error.

	// PROPAGATE: name null bubbles to the nullable hero -> hero:null.
	t.Run("PROPAGATE bubbles to nullable hero", runWithoutError(onErrorE2ECase(t, resolve.ErrorBehaviorPropagate,
		`{"data":{"hero":null},"errors":[{"message":"Failed to fetch from Subgraph 'id'."},{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}]}`)))

	// NULL: name set to null in place, sibling id preserved, no propagation.
	t.Run("NULL keeps sibling data", runWithoutError(onErrorE2ECase(t, resolve.ErrorBehaviorNull,
		`{"data":{"hero":{"id":"1","name":null}},"errors":[{"message":"Failed to fetch from Subgraph 'id'."},{"message":"Cannot return null for non-nullable field 'Query.hero.name'.","path":["hero","name"]}]}`)))

	// HALT: data null with exactly one error.
	t.Run("HALT nulls data with single error", runWithoutError(onErrorE2ECase(t, resolve.ErrorBehaviorHalt,
		`{"data":null,"errors":[{"message":"Failed to fetch from Subgraph 'id'."}]}`)))
}
