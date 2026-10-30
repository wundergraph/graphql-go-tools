package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_DeferMutation(t *testing.T) {
	definition := `
		type Thing {
			id: ID!
			slow: String!
			owner: User!
		}

		type User {
			id: ID!
			title: String!
			lastName: String!
		}

		type Query {
			thing: Thing!
		}

		type Mutation {
			createThing: Thing!
			createUser: User!
		}
	`

	mutationSubgraphSDL := `
		type Thing {
			id: ID!
			slow: String!
			owner: User!
		}

		type User @key(fields: "id") {
			id: ID!
			title: String!
		}

		type Query {
			thing: Thing!
		}

		type Mutation {
			createThing: Thing!
			createUser: User!
		}
	`

	usersSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String!
		}
	`

	schema, err := graphql.NewSchemaFromString(definition)
	require.NoError(t, err)

	dataSources := func(t *testing.T, rec *multiFetchRecorder) []plan.DataSource {
		t.Helper()

		return []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"mutation-service",
				mustFactory(t, recordingClient(t, rec, "mutation", "/", map[string]sendResponse{
					`{"query":"mutation{a: createThing {id slow}}"}`: {
						statusCode: 200,
						body:       `{"data":{"a":{"id":"t1","slow":"done"}}}`,
					},
					`{"query":"mutation{u: createUser {title __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"u":{"title":"Dr","__typename":"User","id":"u1"}}}`,
					},
					`{"query":"mutation{u: createUser {id title __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"u":{"id":"u1","title":"Dr","__typename":"User"}}}`,
					},
					`{"query":"mutation{a: createThing {id owner {id title __typename __internal_id: id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"a":{"id":"t1","owner":{"id":"u1","title":"Dr","__typename":"User","__internal_id":"u1"}}}}`,
					},
				})),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"thing"}},
						{TypeName: "Mutation", FieldNames: []string{"createThing", "createUser"}},
						{TypeName: "User", FieldNames: []string{"id", "title"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "Thing", FieldNames: []string{"id", "slow", "owner"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "User", SelectionSet: "id"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{URL: "https://mutation/", Method: "POST"},
					SchemaConfiguration: mustSchemaConfig(t,
						&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: mutationSubgraphSDL},
						mutationSubgraphSDL,
					),
				}),
			),
			mustGraphqlDataSourceConfiguration(t,
				"users-service",
				mustFactory(t, recordingClient(t, rec, "users", "/", map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","lastName":"Smith"}]}}`,
					},
				})),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "User", FieldNames: []string{"id", "lastName"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "User", SelectionSet: "id"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{URL: "https://users/", Method: "POST"},
					SchemaConfiguration: mustSchemaConfig(t,
						&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: usersSubgraphSDL},
						usersSubgraphSDL,
					),
				}),
			),
		}
	}

	t.Run("[E1] non-entity defer runs the mutation once and returns one result", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateThing",
					Query: `
						mutation CreateThing {
							a: createThing {
								id
								... @defer {
									slow
								}
							}
						}`,
				}
			},
			dataSources:      dataSources(t, rec),
			expectedResponse: `{"data":{"a":{"id":"t1","slow":"done"}}}`,
		})(t)

		assert.Equal(t, []string{
			`{"query":"mutation{a: createThing {id slow}}"}`,
		}, rec.requests())
	})

	t.Run("[E2] two mutation root fields run once each and keep only the entity-backed defer", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateBoth",
					Query: `
						mutation CreateBoth {
							a: createThing {
								id
								... @defer {
									slow
								}
							}
							u: createUser {
								title
								... @defer {
									lastName
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"a":{"id":"t1","slow":"done"},"u":{"title":"Dr"}},"pending":[{"id":"2","path":["u"]}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{a: createThing {id slow}}"}`,
			`{"query":"mutation{u: createUser {title __typename id}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E3] unsafe nested defer joins the initial response and the outer defer stays", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateUser",
					Query: `
						mutation CreateUser {
							u: createUser {
								id
								... @defer(label: "outer") {
									lastName
									... @defer(label: "inner") {
										title
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1","title":"Dr"}},"pending":[{"id":"1","path":["u"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id title __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E4] one defer splits per field and keeps only the entity-backed field deferred", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateThing",
					Query: `
						mutation CreateThing {
							a: createThing {
								id
								... @defer {
									owner {
										id
										title
										lastName
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"a":{"id":"t1","owner":{"id":"u1","title":"Dr"}}},"pending":[{"id":"1","path":["a"]}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"1","subPath":["owner"]}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{a: createThing {id owner {id title __typename __internal_id: id}}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E5] response announces only the surviving inner defer", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateUser",
					Query: `
						mutation CreateUser {
							u: createUser {
								id
								... @defer(label: "outer") {
									title
									... @defer(label: "inner") {
										lastName
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1","title":"Dr"}},"pending":[{"id":"2","path":["u"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id title __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})
}
