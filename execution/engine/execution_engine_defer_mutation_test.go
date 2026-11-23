package engine

import (
	"slices"
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
			info: Info!
		}

		type Info {
			a: String!
			b: String!
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
			info: Info! @shareable
		}

		type Info {
			a: String!
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
			info: Info! @shareable
		}

		type Info {
			b: String!
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
					`{"query":"mutation{u: createUser {id __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"u":{"id":"u1","__typename":"User"}}}`,
					},
					`{"query":"mutation{u: createUser {id info {__internal_typename: __typename} __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"u":{"id":"u1","info":{"__internal_typename":"Info"},"__typename":"User"}}}`,
					},
					`{"query":"mutation{a: createThing {id owner {__typename __internal_id: id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"a":{"id":"t1","owner":{"__typename":"User","__internal_id":"u1"}}}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","title":"Dr"}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","id":"u1","title":"Dr"}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","info":{"a":"A"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title info {__internal_typename: __typename}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","title":"Dr","info":{"__internal_typename":"Info"}}]}}`,
					},
				})),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"thing"}},
						{TypeName: "Mutation", FieldNames: []string{"createThing", "createUser"}},
						{TypeName: "User", FieldNames: []string{"id", "title", "info"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "Thing", FieldNames: []string{"id", "slow", "owner"}},
						{TypeName: "Info", FieldNames: []string{"a"}},
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
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","info":{"b":"B"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {__internal_typename: __typename}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","info":{"__internal_typename":"Info"}}]}}`,
					},
				})),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "User", FieldNames: []string{"id", "lastName", "info"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "Info", FieldNames: []string{"b"}},
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
			expectedResponse: `{"data":{"u":{"id":"u1"}},"pending":[{"id":"1","path":["u"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["u"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"title":"Dr"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E4] one defer splits per field: the non-entity parent goes to the initial response, its entity fields stay deferred", func(t *testing.T) {
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
			expectedResponse: `{"data":{"a":{"id":"t1","owner":{}}},"pending":[{"id":"1","path":["a"]}],"hasNext":true}
{"incremental":[{"data":{"id":"u1","title":"Dr","lastName":"Smith"},"id":"1","subPath":["owner"]}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		// The two entity fetches of the defer run in parallel, so the test sorts the requests.
		requests := rec.requests()
		slices.Sort(requests)
		assert.Equal(t, []string{
			`{"query":"mutation{a: createThing {id owner {__typename __internal_id: id}}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename id title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, requests)
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
			expectedResponse: `{"data":{"u":{"id":"u1"}},"pending":[{"id":"1","path":["u"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"title":"Dr"},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["u"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"lastName":"Smith"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	requiresDefinition := `
		type User {
			id: ID!
			lastName: String!
			fullName: FullName!
		}

		type FullName {
			title: String!
			slow: String!
		}

		type Query {
			user: User!
		}

		type Mutation {
			createUser: User!
		}
	`

	firstSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String! @external
			fullName: FullName! @requires(fields: "lastName")
		}

		type FullName {
			title: String!
			slow: String!
		}

		type Query {
			user: User!
		}

		type Mutation {
			createUser: User!
		}
	`

	secondSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			lastName: String!
		}
	`

	requiresSchema, err := graphql.NewSchemaFromString(requiresDefinition)
	require.NoError(t, err)

	requiresDataSources := func(t *testing.T, rec *multiFetchRecorder) []plan.DataSource {
		t.Helper()

		return []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"first-service",
				mustFactory(t, recordingClient(t, rec, "first", "/", map[string]sendResponse{
					`{"query":"mutation{createUser {id __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"createUser":{"id":"u1","__typename":"User"}}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[{"__typename":"User","lastName":"Smith","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","fullName":{"title":"Dr"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[{"__typename":"User","lastName":"Smith","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","fullName":{"slow":"done"}}]}}`,
					},
				})),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"user"}},
						{TypeName: "Mutation", FieldNames: []string{"createUser"}},
						{TypeName: "User", FieldNames: []string{"id", "fullName"}},
					},
					ChildNodes: []plan.TypeField{
						{TypeName: "FullName", FieldNames: []string{"title", "slow"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "User", SelectionSet: "id"},
						},
						Requires: plan.FederationFieldConfigurations{
							{TypeName: "User", FieldName: "fullName", SelectionSet: "lastName"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{URL: "https://first/", Method: "POST"},
					SchemaConfiguration: mustSchemaConfig(t,
						&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: firstSubgraphSDL},
						firstSubgraphSDL,
					),
				}),
			),
			mustGraphqlDataSourceConfiguration(t,
				"second-service",
				mustFactory(t, recordingClient(t, rec, "second", "/", map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_lastName: lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_lastName":"Smith"}]}}`,
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
					Fetch: &graphql_datasource.FetchConfiguration{URL: "https://second/", Method: "POST"},
					SchemaConfiguration: mustSchemaConfig(t,
						&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: secondSubgraphSDL},
						secondSubgraphSDL,
					),
				}),
			),
		}
	}

	t.Run("[E6] nested defer below a deferred @requires field runs the mutation once", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		runWithoutError(ExecutionEngineTestCase{
			schema: requiresSchema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateUser",
					Query: `
						mutation CreateUser {
							createUser {
								id
								... @defer {
									fullName {
										title
										... @defer {
											slow
										}
									}
								}
							}
						}`,
				}
			},
			dataSources: requiresDataSources(t, rec),
			expectedResponse: `{"data":{"createUser":{"id":"u1"}},"pending":[{"id":"1","path":["createUser"]}],"hasNext":true}
{"incremental":[{"data":{"fullName":{"title":"Dr"}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["createUser","fullName"]}],"hasNext":true}
{"incremental":[{"data":{"slow":"done"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_lastName: lastName}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {title}}}}","variables":{"representations":[{"__typename":"User","lastName":"Smith","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename fullName {slow}}}}","variables":{"representations":[{"__typename":"User","lastName":"Smith","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E7] shareable field below a mutation root field arrives whole in the deferred payload", func(t *testing.T) {
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
								... @defer {
									info {
										a
										b
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1"}},"pending":[{"id":"1","path":["u"]}],"hasNext":true}
{"incremental":[{"data":{"info":{"a":"A","b":"B"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		// The two entity fetches of the defer run in parallel, so the test sorts the requests.
		requests := rec.requests()
		slices.Sort(requests)
		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, requests)
	})

	t.Run("[E8] deferred field of a value type arrives through an entity fetch at the nearest entity above it", func(t *testing.T) {
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
								info {
									... @defer {
										a
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1","info":{}}},"pending":[{"id":"1","path":["u","info"]}],"hasNext":true}
{"incremental":[{"data":{"a":"A"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id info {__internal_typename: __typename} __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E9] shareable field without a key jump on the mutation subgraph splits between the initial and the deferred payload", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		unresolvableMutationSubgraphSDL := `
			type User @key(fields: "id", resolvable: false) {
				id: ID!
				title: String!
				info: Info! @shareable
			}

			type Info {
				a: String!
			}

			type Mutation {
				createUser: User!
			}
		`

		runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateUser",
					Query: `
						mutation CreateUser {
							u: createUser {
								id
								... @defer {
									info {
										a
										b
									}
								}
							}
						}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"mutation-service",
					mustFactory(t, recordingClient(t, rec, "mutation", "/", map[string]sendResponse{
						`{"query":"mutation{u: createUser {id info {a} __typename}}"}`: {
							statusCode: 200,
							body:       `{"data":{"u":{"id":"u1","info":{"a":"A"},"__typename":"User"}}}`,
						},
					})),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Mutation", FieldNames: []string{"createUser"}},
							{TypeName: "User", FieldNames: []string{"id", "title", "info"}},
						},
						ChildNodes: []plan.TypeField{
							{TypeName: "Info", FieldNames: []string{"a"}},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{TypeName: "User", SelectionSet: "id", DisableEntityResolver: true},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{URL: "https://mutation/", Method: "POST"},
						SchemaConfiguration: mustSchemaConfig(t,
							&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: unresolvableMutationSubgraphSDL},
							unresolvableMutationSubgraphSDL,
						),
					}),
				),
				dataSources(t, rec)[1],
			},
			expectedResponse: `{"data":{"u":{"id":"u1","info":{"a":"A"}}},"pending":[{"id":"1","path":["u"]}],"hasNext":true}
{"incremental":[{"data":{"b":"B"},"id":"1","subPath":["info"]}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id info {a} __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E10] inner defer below a value type in a jumped outer defer gets its own entity fetch", func(t *testing.T) {
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
									info {
										... @defer(label: "inner") {
											a
										}
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1"}},"pending":[{"id":"1","path":["u"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"title":"Dr","info":{}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["u","info"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"a":"A"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title info {__internal_typename: __typename}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {a}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E11] inner defer from the other subgraph below a value type in a jumped outer defer", func(t *testing.T) {
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
									info {
										... @defer(label: "inner") {
											b
										}
									}
								}
							}
						}`,
				}
			},
			dataSources: dataSources(t, rec),
			expectedResponse: `{"data":{"u":{"id":"u1"}},"pending":[{"id":"1","path":["u"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"title":"Dr","info":{}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["u","info"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"b":"B"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		// The two entity fetches of the outer defer run in parallel, so the test sorts the requests.
		requests := rec.requests()
		slices.Sort(requests)
		assert.Equal(t, []string{
			`{"query":"mutation{u: createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {__internal_typename: __typename}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {b}}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, requests)
	})

	t.Run("[E12] defer below a mutation root field of an interface object jumps back into the interface object", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		interfaceObjectDefinition := `
			interface Account {
				id: ID!
				title: String!
			}

			type User implements Account {
				id: ID!
				title: String!
			}

			type Query {
				account: Account
			}

			type Mutation {
				createAccount: Account!
			}
		`

		interfaceObjectSubgraphSDL := `
			type Account @key(fields: "id") @interfaceObject {
				id: ID!
				title: String!
			}

			type Mutation {
				createAccount: Account!
			}
		`

		entityInterfaceSubgraphSDL := `
			interface Account @key(fields: "id") {
				id: ID!
			}

			type User implements Account @key(fields: "id") {
				id: ID!
			}

			type Query {
				account: Account
			}
		`

		interfaceObjectSchema, err := graphql.NewSchemaFromString(interfaceObjectDefinition)
		require.NoError(t, err)

		runWithoutError(ExecutionEngineTestCase{
			schema: interfaceObjectSchema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateAccount",
					Query: `
						mutation CreateAccount {
							createAccount {
								id
								... @defer {
									title
								}
							}
						}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"interface-object-service",
					mustFactory(t, recordingClient(t, rec, "interface-object", "/", map[string]sendResponse{
						`{"query":"mutation{createAccount {__typename id}}"}`: {
							statusCode: 200,
							body:       `{"data":{"createAccount":{"__typename":"Account","id":"a1"}}}`,
						},
						`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename title}}}","variables":{"representations":[{"__typename":"Account","id":"a1"}]}}`: {
							statusCode: 200,
							body:       `{"data":{"_entities":[{"__typename":"Account","title":"Admin"}]}}`,
						},
					})),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Mutation", FieldNames: []string{"createAccount"}},
							{TypeName: "Account", FieldNames: []string{"id", "title"}},
							{TypeName: "User", FieldNames: []string{"id", "title"}},
						},
						FederationMetaData: plan.FederationMetaData{
							InterfaceObjects: []plan.EntityInterfaceConfiguration{
								{InterfaceTypeName: "Account", ConcreteTypeNames: []string{"User"}},
							},
							Keys: plan.FederationFieldConfigurations{
								{TypeName: "Account", SelectionSet: "id"},
								{TypeName: "User", SelectionSet: "id"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{URL: "https://interface-object/", Method: "POST"},
						SchemaConfiguration: mustSchemaConfig(t,
							&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: interfaceObjectSubgraphSDL},
							interfaceObjectSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"entity-interface-service",
					mustFactory(t, recordingClient(t, rec, "entity-interface", "/", map[string]sendResponse{})),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Query", FieldNames: []string{"account"}},
							{TypeName: "Account", FieldNames: []string{"id"}},
							{TypeName: "User", FieldNames: []string{"id"}},
						},
						FederationMetaData: plan.FederationMetaData{
							EntityInterfaces: []plan.EntityInterfaceConfiguration{
								{InterfaceTypeName: "Account", ConcreteTypeNames: []string{"User"}},
							},
							Keys: plan.FederationFieldConfigurations{
								{TypeName: "Account", SelectionSet: "id"},
								{TypeName: "User", SelectionSet: "id"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{URL: "https://entity-interface/", Method: "POST"},
						SchemaConfiguration: mustSchemaConfig(t,
							&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: entityInterfaceSubgraphSDL},
							entityInterfaceSubgraphSDL,
						),
					}),
				),
			},
			expectedResponse: `{"data":{"createAccount":{"id":"a1"}},"pending":[{"id":"1","path":["createAccount"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Admin"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{createAccount {__typename id}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Account {__typename title}}}","variables":{"representations":[{"__typename":"Account","id":"a1"}]}}`,
		}, rec.requests())
	})

	t.Run("[E13] defer key jump skips a key with an external field on the mutation subgraph", func(t *testing.T) {
		rec := &multiFetchRecorder{}

		externalKeyDefinition := `
			type User {
				id: ID!
				sku: ID!
				title: String!
			}

			type Query {
				user: User
			}

			type Mutation {
				createUser: User!
			}
		`

		externalKeyMutationSubgraphSDL := `
			type User @key(fields: "sku") @key(fields: "id") {
				sku: ID! @external
				id: ID!
				title: String!
			}

			type Mutation {
				createUser: User!
			}
		`

		skuSubgraphSDL := `
			type User @key(fields: "id") {
				id: ID!
				sku: ID!
			}

			type Query {
				user: User
			}
		`

		externalKeySchema, err := graphql.NewSchemaFromString(externalKeyDefinition)
		require.NoError(t, err)

		runWithoutError(ExecutionEngineTestCase{
			schema: externalKeySchema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "CreateUser",
					Query: `
						mutation CreateUser {
							createUser {
								id
								... @defer {
									title
								}
							}
						}`,
				}
			},
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"mutation-service",
					mustFactory(t, recordingClient(t, rec, "mutation", "/", map[string]sendResponse{
						`{"query":"mutation{createUser {id __typename}}"}`: {
							statusCode: 200,
							body:       `{"data":{"createUser":{"id":"u1","__typename":"User"}}}`,
						},
						`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`: {
							statusCode: 200,
							body:       `{"data":{"_entities":[{"__typename":"User","title":"Dr"}]}}`,
						},
					})),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Mutation", FieldNames: []string{"createUser"}},
							{TypeName: "User", FieldNames: []string{"id", "title"}, ExternalFieldNames: []string{"sku"}},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{TypeName: "User", SelectionSet: "sku"},
								{TypeName: "User", SelectionSet: "id"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{URL: "https://mutation/", Method: "POST"},
						SchemaConfiguration: mustSchemaConfig(t,
							&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: externalKeyMutationSubgraphSDL},
							externalKeyMutationSubgraphSDL,
						),
					}),
				),
				mustGraphqlDataSourceConfiguration(t,
					"sku-service",
					mustFactory(t, recordingClient(t, rec, "sku", "/", map[string]sendResponse{})),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{TypeName: "Query", FieldNames: []string{"user"}},
							{TypeName: "User", FieldNames: []string{"id", "sku"}},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: plan.FederationFieldConfigurations{
								{TypeName: "User", SelectionSet: "id"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{URL: "https://sku/", Method: "POST"},
						SchemaConfiguration: mustSchemaConfig(t,
							&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: skuSubgraphSDL},
							skuSubgraphSDL,
						),
					}),
				),
			},
			expectedResponse: `{"data":{"createUser":{"id":"u1"}},"pending":[{"id":"1","path":["createUser"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Dr"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse())(t)

		assert.Equal(t, []string{
			`{"query":"mutation{createUser {id __typename}}"}`,
			`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"u1"}]}}`,
		}, rec.requests())
	})
}
