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
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{user {__typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","id":"1","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {id __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","id":"1","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {__typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {___typename: __typename __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__typename":"User","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {___typename: __typename __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__typename":"User","id":"1","info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
							},
							`{"query":"{user {info {___typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"___typename":"Info"}}}}`,
							},
							`{"query":"{user {__typename id __internal_1___typename: __typename __internal_1_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","id":"1","__internal_1___typename":"User","__internal_1_id":"1"}}}`,
							},
							`{"query":"{user {id __typename __internal_1___typename: __typename __internal_1_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1","__typename":"User","__internal_1___typename":"User","__internal_1_id":"1"}}}`,
							},
							`{"query":"{user {___typename: __typename __internal_1___typename: __typename __internal_1_id: id __internal_2___typename: __typename __internal_2_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"___typename":"User","__internal_1___typename":"User","__internal_1_id":"1","__internal_2___typename":"User","__internal_2_id":"1"}}}`,
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
						expectedHost: "second",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
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
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name info {___typename: __typename}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {email phone}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat","info":{"phone":"123"}}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name info {email}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
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
}
