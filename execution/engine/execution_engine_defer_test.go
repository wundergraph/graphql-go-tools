package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Execute_Defer(t *testing.T) {
	type TestCase struct {
		name        string
		definition  string
		dataSources []plan.DataSource
	}

	makeRootNodesTestCase := func() TestCase {
		definition := `
				type User {
					id: ID!
					name: String!
					title: String!
					info: Info!
				}

				type Info {
					email: String!
					phone: String!
				}

				type Query {
					user: User!
				}
			`

		dataSources := []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id-1",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: true,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{user {name}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black"}}}`,
							},
							`{"query":"{user {___typename: __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User"}}}`,
							},
							`{"query":"{user {title}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"title":"Sabbat"}}}`,
							},
							`{"query":"{user {id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1"}}}`,
							},
							`{"query":"{user {title id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"title":"Sabbat","id":"1"}}}`,
							},
							`{"query":"{user {name title id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black","title":"Sabbat","id":"1"}}}`,
							},
							`{"query":"{user {info {email phone}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat","phone":"123"}}}}`,
							},
							`{"query":"{user {info {phone} title}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"phone":"123"},"title":"Sabbat"}}}`,
							},
							`{"query":"{user {name info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {name info {___typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black","info":{"___typename":"Info"}}}}`,
							},
							`{"query":"{user {info {___typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"___typename":"Info"}}}}`,
							},
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {info {phone}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"phone":"123"}}}}`,
							},
						},
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"user"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "title", "name", "info"},
						},
						{
							TypeName:   "Info",
							FieldNames: []string{"email", "phone"},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://first/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphql_datasource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: definition,
						},
						definition,
					),
				}),
			),
		}

		return TestCase{
			name:        "defer on non entity field",
			definition:  definition,
			dataSources: dataSources,
		}
	}

	makeEntityTestCase := func() TestCase {
		definition := `
				type User {
					id: ID!
					name: String!
					title: String!
					info: Info!
				}

				type Info {
					email: String!
					phone: String!
				}

				type Query {
					user: User!
				}
			`

		firstSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					info: Info!
				}

				type Info {
					email: String!
				}

				type Query {
					user: User!
				}
			`

		secondSubgraphSDL := `
				type User @key(fields: "id") {
					id: ID!
					name: String!
					title: String!
					info: Info!
				}

				type Info {
					phone: String!
				}
			`

		dataSources := []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id-1",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: true,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{user {id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {___typename: __typename __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__typename":"User","id":1}}}`,
							},
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {info {___typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"___typename":"Info"}}}}`,
							},
							`{"query":"{user {__typename __internal_id: id __internal_1_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","__internal_id":"1","__internal_1_id":"1"}}}`,
							},
							`{"query":"{user {info {___typename: __typename} __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"___typename":"Info"},"__typename":"User","id":"1"}}}`,
							},
							`{"query":"{user {___typename: __typename __typename __internal_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__typename":"User","__internal_id":"1"}}}`,
							},
							`{"query":"{user {___typename: __typename __typename __internal_id: id __internal_4_id: id __internal_5_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__typename":"User","__internal_id":"1","__internal_4_id":"1","__internal_5_id":"1"}}}`,
							},
							`{"query":"{user {__typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","id":"1"}}}`,
							},
							`{"query":"{user {id __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1","__typename":"User"}}}`,
							},
							`{"query":"{user {info {email} __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"},"__typename":"User","id":"1"}}}`,
							},
						},
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"user"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "info"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Info",
							FieldNames: []string{"email"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://first/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphql_datasource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: firstSubgraphSDL,
						},
						firstSubgraphSDL,
					),
				}),
			),
			mustGraphqlDataSourceConfiguration(t,
				"id-2",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: true,
						expectedHost: "second",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[{"__typename":"User","id":1}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black"}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":1}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","title":"Sabbat"}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {phone} title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {phone}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
						},
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "title", "name", "info"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Info",
							FieldNames: []string{"phone"},
						},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{
								TypeName:     "User",
								SelectionSet: "id",
							},
						},
					},
				},
				mustConfiguration(t, graphql_datasource.ConfigurationInput{
					Fetch: &graphql_datasource.FetchConfiguration{
						URL:    "https://second/",
						Method: "POST",
					},
					SchemaConfiguration: mustSchemaConfig(
						t,
						&graphql_datasource.FederationConfiguration{
							Enabled:    true,
							ServiceSDL: secondSubgraphSDL,
						},
						secondSubgraphSDL,
					),
				}),
			),
		}

		return TestCase{
			name:        "entity - distributed fields",
			definition:  definition,
			dataSources: dataSources,
		}
	}

	testCases := []TestCase{
		makeRootNodesTestCase(),
		makeEntityTestCase(),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			schema, err := graphql.NewSchemaFromString(tc.definition)
			require.NoError(t, err)

			t.Run("single deffered field", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer {
									title
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("single deffered field between regular fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								title
								... @defer {
									name
								}
								id
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"title":"Sabbat","id":"1"}},"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("multiple deffered fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer {
									title
									id
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat","id":"1"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("multiple deffered fields - all object fields deferred", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								... @defer {
									name
									title
									id
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Black","title":"Sabbat","id":"1"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("nested defers", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer {
									title
									... @defer {
										id
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("nested defers variation", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserNameTitle",
						Query: `
						query DeferUserNameTitle {
							user {
								... @defer {
									name
									... @defer { title }
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("parallel defers", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer {
									title
								}
								... @defer {
									id
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer {
									info {
										email
										phone
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"hasNext":true}
{"incremental":[{"data":{"info":{"email":"black@sabbat","phone":"123"}},"path":["user"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object with duplicated non defered object", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								info {
									email
								}
								... @defer {
									info {
										phone
									}
									title
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black","info":{"email":"black@sabbat"}}},"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]},{"data":{"phone":"123"},"path":["user","info"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								info {
									... @defer {
										email
										phone
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black","info":{}}},"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat","phone":"123"},"path":["user","info"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("extensive parallel defers across all possible fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferEverythingParallel",
						Query: `
						query DeferEverythingParallel {
							... @defer {
								user {
									... @defer { id }
									... @defer { name }
									... @defer { title }
									... @defer {
										info {
											... @defer { email }
											... @defer { phone }
										}
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{},"hasNext":true}
{"incremental":[{"data":{"user":{}},"path":[]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"path":["user","info"]}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("extensive fully nested defers across all possible fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferEverythingNested",
						Query: `
						query DeferEverythingNested {
							... @defer {
								user {
									... @defer {
										id
										... @defer {
											name
											... @defer {
												title
												... @defer {
													info {
														... @defer {
															email
															... @defer {
																phone
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{},"hasNext":true}
{"incremental":[{"data":{"user":{}},"path":[]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"path":["user","info"]}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})
	}

	t.Run("cross subgraph requires", func(t *testing.T) {
		// Merged schema visible to clients.
		definition := `
		type Query {
			user: User!
		}
		type User {
			id: ID!
			name: String!
			billing: Billing!
			settings: Settings!
			account: Account!
			notifications: [String!]!
		}
		type Billing {
			plan: String!
			currency: String!
		}
		type Settings {
			region: String!
			language: String!
		}
		type Account {
			type: String!
			limit: Int!
		}
	`

		// Subgraph 1: owns Query.user, User.name, User.account.
		// account @requires(fields: "billing { plan } settings { region }") — depends on sub2 and sub3.
		firstSubgraphSDL := `
		type Query {
			user: User!
		}
		
		type User @key(fields: "id") {
			id: ID!
			name: String!
			account: Account! @requires(fields: "billing { plan } settings { region }")
			billing: Billing! @external
			settings: Settings! @external
		}
		
		type Account {
			type: String!
			limit: Int!
		}
		
		type Billing {
			plan: String! @external
		}
		
		type Settings {
			region: String! @external
		}
	`

		// Subgraph 2: owns User.billing, User.notifications.
		// notifications @requires(fields: "name settings { language }") — depends on sub1 (name) and sub3 (settings).
		secondSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			name: String! @external
			notifications: [String!]! @requires(fields: "name settings { language }")
			billing: Billing!
			settings: Settings! @external
		}
		
		type Billing {
			plan: String!
			currency: String!
		}
		
		type Settings {
			language: String! @external
		}
	`

		// Subgraph 3: owns User.settings.
		thirdSubgraphSDL := `
		type User @key(fields: "id") {
			id: ID!
			settings: Settings!
		}
		
		type Settings {
			region: String!
			language: String!
		}
	`

		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		dataSources := []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t, "id-1", mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					// Direct root queries (non-defer, no entity deps)
					`{"query":"{user {name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice"}}}`,
					}, // Initial root query when only entity key is needed (account @requires billing+settings from sub2/sub3)
					`{"query":"{user {__typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"__typename":"User","id":"1"}}}`,
					},
					`{"query":"{user {___typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"___typename":"User"}}}`,
					},
					`{"query":"{user {name __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice","__typename":"User","id":"1"}}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename account {type}}}}","variables":{"representations":[{"__typename":"User","billing":{"plan":"pro"},"settings":{"region":"us-east"},"id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","account":{"type":"premium"}}]}}`,
					}, // Direct root queries for deferred account/name fields
					`{"query":"{user {account {type}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"account":{"type":"premium"}}}}`,
					},
					`{"query":"{user {__internal_name: name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"__internal_name":"Alice"}}}`,
					},
					`{"query":"{user {name account {type} __internal_name: name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice","account":{"type":"premium"},"__internal_name":"Alice"}}}`,
					},
					`{"query":"{user {___typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"___typename":"User","__typename":"User","id":"1"}}}`,
					}, // Deferred sub1 root fetch without redundant plain name (covered by __internal_name alias)
					`{"query":"{user {account {type} __internal_name: name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"account":{"type":"premium"},"__internal_name":"Alice"}}}`,
					},
				},
			})), &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Query",
						FieldNames: []string{"user"},
					},
					{
						TypeName:           "User",
						FieldNames:         []string{"id", "name", "account"},
						ExternalFieldNames: []string{"billing", "settings"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Account",
						FieldNames: []string{"type", "limit"},
					},
					{
						TypeName:           "Billing",
						ExternalFieldNames: []string{"plan"},
					},
					{
						TypeName:           "Settings",
						ExternalFieldNames: []string{"region"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
					Requires: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							FieldName:    "account",
							SelectionSet: "billing { plan } settings { region }",
						},
					},
				},
			}, mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://first/",
					Method: "POST",
				},
				SchemaConfiguration: mustSchemaConfig(t, &graphql_datasource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: firstSubgraphSDL,
				}, firstSubgraphSDL),
			})),
			mustGraphqlDataSourceConfiguration(t, "id-2", mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "second",
				expectedPath: "/",
				responses: map[string]sendResponse{
					// Entity fetches for billing.plan (needed as @requires input for sub1 account)
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename billing {plan}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","billing":{"plan":"pro"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename notifications}}}","variables":{"representations":[{"__typename":"User","name":"Alice","settings":{"language":"en"},"id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","notifications":["msg1","msg2"]}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_billing: billing {plan}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_billing":{"plan":"pro"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename billing {plan} __internal_billing: billing {plan}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","billing":{"plan":"pro"},"__internal_billing":{"plan":"pro"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename billing {plan} notifications}}}","variables":{"representations":[{"__typename":"User","id":"1","name":"Alice","settings":{"language":"en"}}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","billing":{"plan":"pro"},"notifications":["msg1","msg2"]}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename notifications billing {plan}}}}","variables":{"representations":[{"__typename":"User","id":"1","name":"Alice","settings":{"language":"en"}}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","notifications":["msg1","msg2"],"billing":{"plan":"pro"}}]}}`,
					},
				},
			})), &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:           "User",
						FieldNames:         []string{"id", "billing", "notifications"},
						ExternalFieldNames: []string{"name", "settings"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Billing",
						FieldNames: []string{"plan", "currency"},
					},
					{
						TypeName:           "Settings",
						ExternalFieldNames: []string{"language"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
					Requires: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							FieldName:    "notifications",
							SelectionSet: "name settings { language }",
						},
					},
				},
			}, mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://second/",
					Method: "POST",
				},
				SchemaConfiguration: mustSchemaConfig(t, &graphql_datasource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: secondSubgraphSDL,
				}, secondSubgraphSDL),
			})),
			mustGraphqlDataSourceConfiguration(t, "id-3", mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "third",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_3_settings: settings {language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_3_settings":{"language":"en"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_2_settings: settings {language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_2_settings":{"language":"en"}}]}}`,
					}, // New alias format: simple __internal_settings for first defer scope
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_settings: settings {region}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_settings":{"region":"us-east"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_settings: settings {language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_settings":{"language":"en"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename settings {language} __internal_settings: settings {language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","settings":{"language":"en"},"__internal_settings":{"language":"en"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename settings {region}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","settings":{"region":"us-east"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename settings {language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","settings":{"language":"en"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename settings {region language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","settings":{"region":"us-east","language":"en"}}]}}`,
					}, // Combined region+language with simple alias (new format)
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename __internal_settings: settings {region language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","__internal_settings":{"region":"us-east","language":"en"}}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename settings {region} __internal_settings: settings {region language}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","settings":{"region":"us-east"},"__internal_settings":{"region":"us-east","language":"en"}}]}}`,
					},
				},
			})), &plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "User",
						FieldNames: []string{"id", "settings"},
					},
				},
				ChildNodes: []plan.TypeField{
					{
						TypeName:   "Settings",
						FieldNames: []string{"region", "language"},
					},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{
							TypeName:     "User",
							SelectionSet: "id",
						},
					},
				},
			}, mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://third/",
					Method: "POST",
				},
				SchemaConfiguration: mustSchemaConfig(t, &graphql_datasource.FederationConfiguration{
					Enabled:    true,
					ServiceSDL: thirdSubgraphSDL,
				}, thirdSubgraphSDL),
			})),
		}

		t.Run("non-defer - name only", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}}}`,
		}))

		t.Run("non-defer - account requires billing and settings", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { account { type } } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"account":{"type":"premium"}}}}`,
		}))

		t.Run("non-defer - notifications requires name and settings", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("non-defer - both requires fields together", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name account { type } notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","account":{"type":"premium"},"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("non-defer - all fields including raw billing and settings", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name billing { plan } settings { region } account { type } notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east"},"account":{"type":"premium"},"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("defer - account field deferred", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferAccount",
					Query: `
				query DeferAccount {
					user {
						name
						... @defer {
							account { type }
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - notifications field deferred", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferNotifications",
					Query: `
				query DeferNotifications {
					user {
						name
						... @defer {
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all user fields deferred in single block", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferAll",
					Query: `
				query DeferAll {
					user {
						... @defer {
							name
							account { type }
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Alice","account":{"type":"premium"},"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("all user fields without defer", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferAll",
					Query: `
				query DeferAll {
					user {
						name
						account { type }
						notifications
					}
				}`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","account":{"type":"premium"},"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("defer - parallel defers on both cross-subgraph requires fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferBothRequires",
					Query: `
				query DeferBothRequires {
					user {
						name
						... @defer {
							account { type }
						}
						... @defer {
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - nested defers: outer has account, inner has notifications", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferNested",
					Query: `
				query DeferNested {
					user {
						name
						... @defer {
							account { type }
							... @defer {
								notifications
							}
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - parallel defers on raw entity fields alongside requires", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferMixed",
					Query: `
				query DeferMixed {
					user {
						name
						billing { plan }
						... @defer {
							account { type }
						}
						... @defer {
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"}}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - deeply nested requires: account outer, notifications inner, with raw fields", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferDeepNested",
					Query: `
				query DeferDeepNested {
					user {
						... @defer {
							name
							billing { plan }
							... @defer {
								account { type }
								... @defer {
									notifications
								}
							}
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Alice","billing":{"plan":"pro"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		// Defer versions of each non-defer test — verify @defer doesn't break @requires resolution.

		t.Run("defer - name only", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferNameOnly",
					Query: `
				query DeferNameOnly {
					user {
						... @defer { name }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Alice"},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - only account deferred (no other immediate fields)", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferAccountOnly",
					Query: `
				query DeferAccountOnly {
					user {
						... @defer { account { type } }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - only notifications deferred (no other immediate fields)", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferNotificationsOnly",
					Query: `
				query DeferNotificationsOnly {
					user {
						... @defer { notifications }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all fields in single defer block", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferAllFields",
					Query: `
				query DeferAllFields {
					user {
						... @defer {
							name
							billing { plan }
							settings { region }
							account { type }
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{}},"hasNext":true}
{"incremental":[{"data":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east"},"account":{"type":"premium"},"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		// Tests mixing requires-source fields (billing, settings) with derived @requires fields
		// (account, notifications) in same or parallel defer blocks.

		t.Run("defer - requires source (billing) and derived field (account) in same defer block", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferBillingAndAccount",
					Query: `
				query DeferBillingAndAccount {
					user {
						name
						... @defer {
							billing { plan }
							account { type }
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"},"account":{"type":"premium"}},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (billing) and derived field (account) in parallel defers", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferBillingParallelAccount",
					Query: `
				query DeferBillingParallelAccount {
					user {
						name
						... @defer { billing { plan } }
						... @defer { account { type } }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (settings) and derived field (notifications) in same defer block", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferSettingsAndNotifications",
					Query: `
				query DeferSettingsAndNotifications {
					user {
						name
						... @defer {
							settings { language }
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"settings":{"language":"en"},"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (settings) and derived field (notifications) in parallel defers", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferSettingsParallelNotifications",
					Query: `
				query DeferSettingsParallelNotifications {
					user {
						name
						... @defer { settings { language } }
						... @defer { notifications }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"settings":{"language":"en"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all requires sources deferred together, then derived fields deferred in parallel", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferSourcesThenDerived",
					Query: `
				query DeferSourcesThenDerived {
					user {
						name
						... @defer {
							billing { plan }
							settings { region language }
						}
						... @defer {
							account { type }
							notifications
						}
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"},"settings":{"region":"us-east","language":"en"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"},"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires sources immediate, both derived fields deferred in parallel", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferDerivedFieldsOnly",
					Query: `
				query DeferDerivedFieldsOnly {
					user {
						name
						billing { plan }
						settings { region language }
						... @defer { account { type } }
						... @defer { notifications }
					}
				}`,
				}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east","language":"en"}}},"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"path":["user"]}],"hasNext":false}
`,
		}, withStreamingResponse()))
	})

	t.Run("non-nullable field errors", func(t *testing.T) {
		definition := `
			type Query { product: Product! }
			type Product {
				id: ID!
				name: String!
				nameWithError: String!
				price: Float!
			}
		`

		firstSubgraphSDL := `
			type Query { product: Product! }
			type Product @key(fields: "id") {
				id: ID!
				name: String!
				nameWithError: String
			}
		`

		secondSubgraphSDL := `
			type Product @key(fields: "id") {
				id: ID!
				price: Float!
			}
		`

		dataSources := []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id-1",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: true,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{product {___typename: __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"product":{"___typename":"Product"}}}`,
							},
							`{"query":"{product {___typename: __typename __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"product":{"___typename":"Product","__typename":"Product","id":"1"}}}`,
							},
							`{"query":"{product {name}}"}`: {
								statusCode: 200,
								body:       `{"data":{"product":{"name":null}}}`,
							},
							`{"query":"{product {nameWithError}}"}`: {
								statusCode: 200,
								body:       `{"data":{"product":{"nameWithError":null}},"errors":[{"message":"upstream name error","path":["product","nameWithError"]}]}`,
							},
							`{"query":"{product {name nameWithError}}"}`: {
								statusCode: 200,
								body:       `{"data":{"product":{"name":null,"nameWithError":null}},"errors":[{"message":"upstream name error","path":["product","nameWithError"]}]}`,
							},
						},
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Query", FieldNames: []string{"product"}},
						{TypeName: "Product", FieldNames: []string{"id", "name", "nameWithError"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "Product", SelectionSet: "id"},
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
				"id-2",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: true,
						expectedHost: "second",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename price}}}","variables":{"representations":[{"__typename":"Product","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"Product","price":null}]}}`,
							},
						},
					}),
				),
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{TypeName: "Product", FieldNames: []string{"price"}},
					},
					FederationMetaData: plan.FederationMetaData{
						Keys: plan.FederationFieldConfigurations{
							{TypeName: "Product", SelectionSet: "id"},
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

		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		t.Run("defer from first subgraph - null non-nullable field", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { name } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"hasNext":true}
{"incremental":[{"data":null,"path":["product"],"errors":[{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]},{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from first subgraph - null field with upstream error", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { nameWithError } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"hasNext":true}
{"incremental":[{"data":{"nameWithError":null},"path":["product"],"errors":[{"message":"Failed to fetch from Subgraph 'id-1'."},{"message":"Cannot return null for non-nullable field 'Query.product.nameWithError'.","path":["product","nameWithError"]},{"message":"Cannot return null for non-nullable field 'Query.product.nameWithError'.","path":["product","nameWithError"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from second subgraph - null non-nullable field", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { price } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"hasNext":true}
{"incremental":[{"data": null,"path":["product"],"errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]},{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from both subgraphs - null non-nullable fields - name first", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { name } ... @defer { price } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"hasNext":true}
{"incremental":[{"data":null,"path":["product"],"errors":[{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]},{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from both subgraphs - null non-nullable fields - price first", runExecutionEngineTestWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { price } ... @defer { name } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"hasNext":true}
{"incremental":[{"data": null,"path":["product"],"errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]},{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

	})

}
