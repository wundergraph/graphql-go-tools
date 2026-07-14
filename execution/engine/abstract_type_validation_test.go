package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestAbstractTypeValidation(t *testing.T) {
	t.Parallel()

	schema, err := graphql.NewSchemaFromString(abstractTypeValidationSDL)
	require.NoError(t, err)

	tests := []struct {
		name             string
		fieldName        string
		selection        string
		returnedTypeName string
		serviceSDL       string
		query            string // overrides the generated single-field query when set
		responseBody     string // overrides the generated subgraph response when set
		options          []executionTestOptions
		expectedResponse string
	}{
		{
			name:             "interface accepts an implementation",
			fieldName:        "nullableInterface",
			selection:        "__typename id",
			returnedTypeName: "AccessibleNode",
			expectedResponse: `{"data":{"nullableInterface":{"__typename":"AccessibleNode","id":"1"}}}`,
		},
		{
			name:             "interface rejects an unknown implementation",
			fieldName:        "nullableInterface",
			selection:        "__typename id",
			returnedTypeName: "UnexpectedNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'UnexpectedNode' for __typename field.","path":["nullableInterface"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableInterface":null}}`,
		},
		{
			name:             "interface redacts an inaccessible implementation",
			fieldName:        "nullableInterface",
			selection:        "__typename id",
			returnedTypeName: "InaccessibleNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["nullableInterface"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableInterface":null}}`,
		},
		{
			name:             "non-null interface propagates null for an invalid implementation",
			fieldName:        "interface",
			selection:        "__typename id",
			returnedTypeName: "UnexpectedNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'UnexpectedNode' for __typename field.","path":["interface"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
		},
		{
			name:             "non-null interface redacts an inaccessible implementation and propagates null",
			fieldName:        "interface",
			selection:        "__typename id",
			returnedTypeName: "InaccessibleNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["interface"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
		},
		{
			name:             "union accepts a member",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "AccessibleNode",
			expectedResponse: `{"data":{"nullableUnion":{"__typename":"AccessibleNode","id":"1"}}}`,
		},
		{
			name:             "union rejects an unknown member",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "UnexpectedNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'UnexpectedNode' for __typename field.","path":["nullableUnion"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableUnion":null}}`,
		},
		{
			name:             "union rejects a runtime type that is not a contract member",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "RemovedNode",
			serviceSDL:       abstractTypeValidationSubgraphSDL,
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'RemovedNode' for __typename field.","path":["nullableUnion"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableUnion":null}}`,
		},
		{
			name:             "union redacts an inaccessible member",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "InaccessibleNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["nullableUnion"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableUnion":null}}`,
		},
		{
			name:             "non-null union propagates null for an invalid member",
			fieldName:        "union",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "UnexpectedNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'UnexpectedNode' for __typename field.","path":["union"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
		},
		{
			name:             "non-null union redacts an inaccessible member and propagates null",
			fieldName:        "union",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "InaccessibleNode",
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["union"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":null}`,
		},
		{
			name:             "list reports the index of a rejected unknown element",
			fieldName:        "interfaces",
			selection:        "__typename id",
			responseBody:     `{"data":{"interfaces":[{"__typename":"AccessibleNode","id":"1"},{"__typename":"UnexpectedNode","id":"2"}]}}`,
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned invalid value 'UnexpectedNode' for __typename field.","path":["interfaces",1],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"interfaces":[{"__typename":"AccessibleNode","id":"1"},null]}}`,
		},
		{
			name:             "list reports the index of a redacted inaccessible element",
			fieldName:        "interfaces",
			selection:        "__typename id",
			responseBody:     `{"data":{"interfaces":[{"__typename":"AccessibleNode","id":"1"},{"__typename":"InaccessibleNode","id":"2"}]}}`,
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["interfaces",1],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"interfaces":[{"__typename":"AccessibleNode","id":"1"},null]}}`,
		},
		{
			name:             "list with non-null elements propagates null for an inaccessible element",
			fieldName:        "requiredInterfaces",
			selection:        "__typename id",
			responseBody:     `{"data":{"requiredInterfaces":[{"__typename":"InaccessibleNode","id":"1"}]}}`,
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["requiredInterfaces",0],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"requiredInterfaces":null}}`,
		},
		{
			name:             "abstract fields merged from fragment selections stay validated",
			fieldName:        "nullableInterface",
			query:            `query { nullableInterface { __typename ... on AccessibleNode { friend { __typename id } } ... on SecondNode { friend { __typename id } } } }`,
			responseBody:     `{"data":{"nullableInterface":{"__typename":"AccessibleNode","friend":{"__typename":"InaccessibleNode","id":"1"}}}}`,
			expectedResponse: `{"errors":[{"message":"Subgraph 'AbstractTypes' returned an invalid value for __typename field.","path":["nullableInterface","friend"],"extensions":{"code":"INVALID_GRAPHQL"}}],"data":{"nullableInterface":{"__typename":"AccessibleNode","friend":null}}}`,
		},
		{
			name:             "union rejects an unknown member with value completion",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "UnexpectedNode",
			options:          []executionTestOptions{withValueCompletion()},
			expectedResponse: `{"data":{"nullableUnion":null},"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.nullableUnion.","path":["nullableUnion"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
		},
		{
			name:             "union redacts an inaccessible member with value completion",
			fieldName:        "nullableUnion",
			selection:        "__typename ... on AccessibleNode { id }",
			returnedTypeName: "InaccessibleNode",
			options:          []executionTestOptions{withValueCompletion()},
			expectedResponse: `{"data":{"nullableUnion":null},"extensions":{"valueCompletion":[{"message":"Invalid __typename found for object at field Query.nullableUnion.","path":["nullableUnion"],"extensions":{"code":"INVALID_GRAPHQL"}}]}}`,
		},
	}

	for _, tt := range tests {
		serviceSDL := tt.serviceSDL
		if serviceSDL == "" {
			serviceSDL = abstractTypeValidationSDL
		}
		query := tt.query
		if query == "" {
			query = "query { " + tt.fieldName + " { " + tt.selection + " } }"
		}
		responseBody := tt.responseBody
		if responseBody == "" {
			responseBody = `{"data":{"` + tt.fieldName + `":{"__typename":"` + tt.returnedTypeName + `","id":"1"}}}`
		}

		t.Run(tt.name, runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: query,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfigurationWithName(
						t,
						"abstract-types",
						"AbstractTypes",
						mustFactory(t, testNetHttpClient(t, roundTripperTestCase{
							expectedHost:     "example.com",
							expectedPath:     "/",
							sendResponseBody: responseBody,
							sendStatusCode:   200,
						})),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"interface", "nullableInterface", "union", "nullableUnion", "interfaces", "requiredInterfaces"},
								},
							},
							ChildNodes: []plan.TypeField{
								{TypeName: "Node", FieldNames: []string{"id"}},
								{TypeName: "AccessibleNode", FieldNames: []string{"id", "friend"}},
								{TypeName: "SecondNode", FieldNames: []string{"id", "friend"}},
								{TypeName: "InaccessibleNode", FieldNames: []string{"id"}},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(
								t,
								&graphql_datasource.FederationConfiguration{
									Enabled:    true,
									ServiceSDL: serviceSDL,
								},
								serviceSDL,
							),
						}),
					),
				},
				expectedResponse: tt.expectedResponse,
			},
			tt.options...,
		))
	}
}

const abstractTypeValidationSDL = `
	type Query {
		interface: Node!
		nullableInterface: Node
		union: Result!
		nullableUnion: Result
		interfaces: [Node]
		requiredInterfaces: [Node!]
	}

	interface Node {
		id: ID!
	}

	type AccessibleNode implements Node {
		id: ID!
		friend: Node
	}

	type SecondNode implements Node {
		id: ID!
		friend: Node
	}

	type InaccessibleNode implements Node @inaccessible {
		id: ID!
	}

	union Result = AccessibleNode | InaccessibleNode
`

const abstractTypeValidationSubgraphSDL = abstractTypeValidationSDL + `
	type RemovedNode implements Node {
		id: ID!
	}

	extend union Result = RemovedNode
`
