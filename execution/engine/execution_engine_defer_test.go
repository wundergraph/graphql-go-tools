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

			t.Run("single deffered field", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("single deffered field between regular fields", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"title":"Sabbat","id":"1"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("multiple deffered fields", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat","id":"1"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("multiple deffered fields - all object fields deferred", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black","title":"Sabbat","id":"1"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("nested defers", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("single deffered field with label", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer(label: "titleLabel") {
									title
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"],"label":"titleLabel"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("multiple deffered fields with label", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer(label: "detailsLabel") {
									title
									id
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"],"label":"detailsLabel"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat","id":"1"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("nested defers with labels", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								name
								... @defer(label: "outer") {
									title
									... @defer(label: "inner") {
										id
									}
								}
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"],"label":"outer"},{"id":"2","path":["user"],"label":"inner"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("labeled and unlabeled sibling defers", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						OperationName: "DeferUserTitle",
						Query: `
						query DeferUserTitle {
							user {
								... @defer(label: "a") { title }
								... @defer { id }
							}
						}`,
					}
				},
				dataSources: tc.dataSources,
				expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"],"label":"a"},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("nested defers variation", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("parallel defers", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"info":{"email":"black@sabbat","phone":"123"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object with duplicated non defered object", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black","info":{"email":"black@sabbat"}}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"},{"data":{"phone":"123"},"id":"1","subPath":["info"]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer nested object fields", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{"user":{"name":"Black","info":{}}},"pending":[{"id":"1","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat","phone":"123"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("extensive parallel defers across all possible fields", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["user"]},{"id":"3","path":["user"]},{"id":"4","path":["user"]},{"id":"5","path":["user"]},{"id":"6","path":["user","info"]},{"id":"7","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"user":{}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"3"}],"completed":[{"id":"3"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"4"}],"completed":[{"id":"4"}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"id":"5"}],"completed":[{"id":"5"}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"id":"6"}],"completed":[{"id":"6"}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"7"}],"completed":[{"id":"7"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("extensive fully nested defers across all possible fields", runWithoutError(ExecutionEngineTestCase{
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["user"]},{"id":"3","path":["user"]},{"id":"4","path":["user"]},{"id":"5","path":["user"]},{"id":"6","path":["user","info"]},{"id":"7","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"user":{}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"3"}],"completed":[{"id":"3"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"4"}],"completed":[{"id":"4"}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"id":"5"}],"completed":[{"id":"5"}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"id":"6"}],"completed":[{"id":"6"}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"7"}],"completed":[{"id":"7"}],"hasNext":false}
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
					`{"query":"{user {name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice"}}}`,
					},
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
					},
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
					},
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
					},
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

		t.Run("non-defer - name only", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice"}}}`,
		}))

		t.Run("non-defer - account requires billing and settings", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { account { type } } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"account":{"type":"premium"}}}}`,
		}))

		t.Run("non-defer - notifications requires name and settings", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("non-defer - both requires fields together", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name account { type } notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","account":{"type":"premium"},"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("non-defer - all fields including raw billing and settings", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ user { name billing { plan } settings { region } account { type } notifications } }`,
				}
			},
			dataSources:      dataSources,
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east"},"account":{"type":"premium"},"notifications":["msg1","msg2"]}}}`,
		}))

		t.Run("defer - account field deferred", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - notifications field deferred", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all user fields deferred in single block", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Alice","account":{"type":"premium"},"notifications":["msg1","msg2"]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("all user fields without defer", runWithoutError(ExecutionEngineTestCase{
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

		t.Run("defer - parallel defers on both cross-subgraph requires fields", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - nested defers: outer has account, inner has notifications", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - parallel defers on raw entity fields alongside requires", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"}}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - deeply nested requires: account outer, notifications inner, with raw fields", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]},{"id":"3","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Alice","billing":{"plan":"pro"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"3"}],"completed":[{"id":"3"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		// Defer versions of each non-defer test — verify @defer doesn't break @requires resolution.

		t.Run("defer - name only", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Alice"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - only account deferred (no other immediate fields)", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - only notifications deferred (no other immediate fields)", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all fields in single defer block", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east"},"account":{"type":"premium"},"notifications":["msg1","msg2"]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		// Tests mixing requires-source fields (billing, settings) with derived @requires fields
		// (account, notifications) in same or parallel defer blocks.

		t.Run("defer - requires source (billing) and derived field (account) in same defer block", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"},"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (billing) and derived field (account) in parallel defers", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (settings) and derived field (notifications) in same defer block", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"settings":{"language":"en"},"notifications":["msg1","msg2"]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires source (settings) and derived field (notifications) in parallel defers", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"settings":{"language":"en"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - all requires sources deferred together, then derived fields deferred in parallel", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"billing":{"plan":"pro"},"settings":{"region":"us-east","language":"en"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"},"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer - requires sources immediate, both derived fields deferred in parallel", runWithoutError(ExecutionEngineTestCase{
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
			expectedResponse: `{"data":{"user":{"name":"Alice","billing":{"plan":"pro"},"settings":{"region":"us-east","language":"en"}}},"pending":[{"id":"1","path":["user"]},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"notifications":["msg1","msg2"]},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))
	})

	t.Run("non-nullable field errors", func(t *testing.T) {
		definition := `
			type Query { product: Product! }
			type Product {
				id: ID!
				name: String!
				nameWithError: String
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

		t.Run("defer from first subgraph - null non-nullable field", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { name } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]}],"hasNext":true}
{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from first subgraph - null field with upstream error", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { nameWithError } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]}],"hasNext":true}
{"incremental":[{"data":{"nameWithError":null},"id":"1","errors":[{"message":"Failed to fetch from Subgraph 'id-1'."}]}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from second subgraph - null non-nullable field", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { price } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]}],"hasNext":true}
{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from both subgraphs - null non-nullable fields - name first", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { name } ... @defer { price } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]},{"id":"2","path":["product"]}],"hasNext":true}
{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]}]}],"hasNext":true}
{"completed":[{"id":"2","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer from both subgraphs - null non-nullable fields - price first", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { price } ... @defer { name } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]},{"id":"2","path":["product"]}],"hasNext":true}
{"completed":[{"id":"1","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":true}
{"completed":[{"id":"2","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.name'.","path":["product","name"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("defer error halts subsequent defers - nameWithError then price", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{Query: `{ product { ... @defer { nameWithError } ... @defer { price } } }`}
			},
			dataSources: dataSources,
			expectedResponse: `{"data":{"product":{}},"pending":[{"id":"1","path":["product"]},{"id":"2","path":["product"]}],"hasNext":true}
{"incremental":[{"data":{"nameWithError":null},"id":"1","errors":[{"message":"Failed to fetch from Subgraph 'id-1'."}]}],"completed":[{"id":"1"}],"hasNext":true}
{"completed":[{"id":"2","errors":[{"message":"Cannot return null for non-nullable field 'Query.product.price'.","path":["product","price"]}]}],"hasNext":false}
`,
		}, withStreamingResponse()))

	})

	t.Run("nested list entities", func(t *testing.T) {
		definition := `
			type Query { items: [Item!]! }
			type Item {
				id:       ID!
				name:     String!
				title:    String!
				subItems: [SubItem!]!
			}
			type SubItem {
				id:          ID!
				description: String!
			}
		`
		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		// Sub1: owns Query.items, Item.{id,name,subItems}, SubItem.id
		firstSubgraphSDL := `
			type Query { items: [Item!]! }
			type Item @key(fields: "id") {
				id:       ID!
				name:     String!
				subItems: [SubItem!]!
			}
			type SubItem @key(fields: "id") {
				id: ID!
			}
		`
		firstSubgraphDS := mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"{items {___typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"___typename":"Item","__typename":"Item","id":"1"},{"___typename":"Item","__typename":"Item","id":"2"}]}}`,
					},
					`{"query":"{items {name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"name":"ItemOne"},{"name":"ItemTwo"}]}}`,
					},
					`{"query":"{items {___typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"___typename":"Item"},{"___typename":"Item"}]}}`,
					},
					`{"query":"{items {subItems {___typename: __typename __typename id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"subItems":[{"___typename":"SubItem","__typename":"SubItem","id":"s1"},{"___typename":"SubItem","__typename":"SubItem","id":"s2"}]},{"subItems":[{"___typename":"SubItem","__typename":"SubItem","id":"s3"}]}]}}`,
					},
					`{"query":"{items {id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"id":"1"},{"id":"2"}]}}`,
					},
					`{"query":"{items {id name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"id":"1","name":"ItemOne"},{"id":"2","name":"ItemTwo"}]}}`,
					},
					`{"query":"{items {subItems {id __typename __internal_id: id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"subItems":[{"id":"s1","__typename":"SubItem","__internal_id":"s1"},{"id":"s2","__typename":"SubItem","__internal_id":"s2"}]},{"subItems":[{"id":"s3","__typename":"SubItem","__internal_id":"s3"}]}]}}`,
					},
					`{"query":"{items {___typename: __typename __typename __internal_id: id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"___typename":"Item","__typename":"Item","__internal_id":"1"},{"___typename":"Item","__typename":"Item","__internal_id":"2"}]}}`,
					},
					`{"query":"{items {id __typename __internal_id: id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"id":"1","__typename":"Item","__internal_id":"1"},{"id":"2","__typename":"Item","__internal_id":"2"}]}}`,
					},
					`{"query":"{items {id subItems {id __typename __internal_id: id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"id":"1","subItems":[{"id":"s1","__typename":"SubItem","__internal_id":"s1"},{"id":"s2","__typename":"SubItem","__internal_id":"s2"}]},{"id":"2","subItems":[{"id":"s3","__typename":"SubItem","__internal_id":"s3"}]}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"items"}},
					{TypeName: "Item", FieldNames: []string{"id", "name", "subItems"}},
					{TypeName: "SubItem", FieldNames: []string{"id"}},
				},
				ChildNodes: []plan.TypeField{
					{TypeName: "SubItem", FieldNames: []string{"id"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "Item", SelectionSet: "id"},
						{TypeName: "SubItem", SelectionSet: "id"},
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
		)

		// Sub2: extends Item with title
		secondSubgraphSDL := `
			type Item @key(fields: "id") {
				id:    ID!
				title: String!
			}
		`
		secondSubgraphDS := mustGraphqlDataSourceConfiguration(t,
			"id-2",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "second",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Item {__typename title}}}","variables":{"representations":[{"__typename":"Item","id":"1"},{"__typename":"Item","id":"2"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Item","title":"TitleOne"},{"__typename":"Item","title":"TitleTwo"}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Item", FieldNames: []string{"id", "title"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "Item", SelectionSet: "id"},
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
		)

		// Sub3: extends SubItem with description
		thirdSubgraphSDL := `
			type SubItem @key(fields: "id") {
				id:          ID!
				description: String!
			}
		`
		thirdSubgraphDS := mustGraphqlDataSourceConfiguration(t,
			"id-3",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "third",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on SubItem {__typename description}}}","variables":{"representations":[{"__typename":"SubItem","id":"s1"},{"__typename":"SubItem","id":"s2"},{"__typename":"SubItem","id":"s3"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"SubItem","description":"Desc1"},{"__typename":"SubItem","description":"Desc2"},{"__typename":"SubItem","description":"Desc3"}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "SubItem", FieldNames: []string{"id", "description"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "SubItem", SelectionSet: "id"},
					},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{URL: "https://third/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t,
					&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: thirdSubgraphSDL},
					thirdSubgraphSDL,
				),
			}),
		)

		dataSources := []plan.DataSource{firstSubgraphDS, secondSubgraphDS, thirdSubgraphDS}

		t.Run("category A - no id in initial response", func(t *testing.T) {
			t.Run("defer name from sub1", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { name } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"1","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer title from sub2", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { title } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"1","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer subItems description from sub3", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { subItems { ... @defer { description } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{"subItems":[{},{}]},{"subItems":[{}]}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"1","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"1","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"1","subPath":[1,"subItems",0]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("items subItems and description all in separate nested defers", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { subItems { id ... @defer { description } } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]},{"id":"3","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"3","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"3","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"3","subPath":[1,"subItems",0]}],"completed":[{"id":"3"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})

		t.Run("category B - id deferred with parallel defers", func(t *testing.T) {
			t.Run("defer id only", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { id } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer id and name together", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { id name } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1","name":"ItemOne"},"id":"1","subPath":[0]},{"data":{"id":"2","name":"ItemTwo"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer id in parallel with name", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { id } ... @defer { name } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("defer id in parallel with title (cross-subgraph)", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { id } ... @defer { title } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("parallel defers on subItems id and description", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { id ... @defer { subItems { id } } ... @defer { subItems { description } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"1","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"2","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"2","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"2","subPath":[1,"subItems",0]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})

		t.Run("parallel root defers", func(t *testing.T) {
			t.Run("subItems id then description", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { subItems { id } } } ... @defer { items { subItems { description } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"subItems":[{"id":"s1"},{"id":"s2"}]},{"subItems":[{"id":"s3"}]}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"2","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"2","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"2","subPath":[1,"subItems",0]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})

		t.Run("category C - nested defers", func(t *testing.T) {
			t.Run("outer defer items, inner defer name", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { name } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("outer defer items, inner defer title (cross-subgraph)", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { title } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("outer defer items with subItems, inner defer description", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id subItems { id ... @defer { description } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1","subItems":[{"id":"s1"},{"id":"s2"}]},{"id":"2","subItems":[{"id":"s3"}]}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"2","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"2","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"2","subPath":[1,"subItems",0]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("three-level defer: query to items to subItems", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { subItems { id ... @defer { description } } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]},{"id":"3","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"3","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"3","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"3","subPath":[1,"subItems",0]}],"completed":[{"id":"3"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("three-level defer with cross-subgraph at middle level", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { title subItems { id ... @defer { description } } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]},{"id":"2","path":["items"]},{"id":"3","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne","subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo","subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"3","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"3","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"3","subPath":[1,"subItems",0]}],"completed":[{"id":"3"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})
	})

	t.Run("named fragments with defer", func(t *testing.T) {
		definition := `
			type Query { products: [Product!]! }
			type Product {
				id:    ID!
				sku:   String!
				name:  String!
				price: Float!
			}
		`
		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		firstSubgraphSDL := `
			type Query { products: [Product!]! }
			type Product @key(fields: "id") {
				id:  ID!
				sku: String!
			}
		`
		firstSubgraphDS := mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"{products {___typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"___typename":"Product","__typename":"Product","id":"1"},{"___typename":"Product","__typename":"Product","id":"2"}]}}`,
					},
					`{"query":"{products {___typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"___typename":"Product"},{"___typename":"Product"}]}}`,
					},
					`{"query":"{products {id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"id":"1"},{"id":"2"}]}}`,
					},
					`{"query":"{products {sku}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"sku":"sku-1"},{"sku":"sku-2"}]}}`,
					},
					`{"query":"{products {id sku}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"id":"1","sku":"sku-1"},{"id":"2","sku":"sku-2"}]}}`,
					},
					`{"query":"{products {id __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"id":"1","__typename":"Product"},{"id":"2","__typename":"Product"}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"products"}},
					{TypeName: "Product", FieldNames: []string{"id", "sku"}},
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
		)

		secondSubgraphSDL := `
			type Product @key(fields: "id") {
				id:    ID!
				name:  String!
				price: Float!
			}
		`
		secondSubgraphDS := mustGraphqlDataSourceConfiguration(t,
			"id-2",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: true,
				expectedHost: "second",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename name}}}","variables":{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Product","name":"Product One"},{"__typename":"Product","name":"Product Two"}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename name price}}}","variables":{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Product","name":"Product One","price":9.99},{"__typename":"Product","name":"Product Two","price":19.99}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Product", FieldNames: []string{"id", "name", "price"}},
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
		)

		dataSources := []plan.DataSource{firstSubgraphDS, secondSubgraphDS}

		t.Run("category A - defer on named fragment spread", func(t *testing.T) {
			t.Run("A1 - defer sub1 field sku via fragment spread", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment SkuFields on Product { sku } { products { ...SkuFields @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("A2 - defer sub2 field name via fragment spread", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment NameFields on Product { name } { products { ...NameFields @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"1","subPath":[0]},{"data":{"name":"Product Two"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("A3 - id non-deferred, sub2 name and price deferred via fragment", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment DetailFields on Product { name price } { products { id ...DetailFields @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One","price":9.99},"id":"1","subPath":[0]},{"data":{"name":"Product Two","price":19.99},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("A4 - parallel fragment spreads from different subgraphs, both deferred", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment SkuFrag on Product { sku } fragment NameFrag on Product { name } { products { ...SkuFrag @defer ...NameFrag @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})

		t.Run("category B - defer inside named fragment definition", func(t *testing.T) {
			t.Run("B1 - defer sub1 field sku inside named fragment", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment ProductFrag on Product { id ... @defer { sku } } { products { ...ProductFrag } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("B2 - defer sub2 field name inside named fragment", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment ProductFrag on Product { id ... @defer { name } } { products { ...ProductFrag } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"1","subPath":[0]},{"data":{"name":"Product Two"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("B3 - parallel sub1 and sub2 defers inside named fragment", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment ProductFrag on Product { id ... @defer { sku } ... @defer { name } } { products { ...ProductFrag } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})

		t.Run("category C - defer on spread containing inner defers", func(t *testing.T) {
			t.Run("C1 - multiple sub1 fields id and sku bundled in single deferred spread", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment SkuIdFrag on Product { id sku } { products { ...SkuIdFrag @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1","sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"id":"2","sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("C2 - outer spread deferred delivering sub1 sku, with nested inner sub2 name defer", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `fragment SkuWithName on Product { sku ... @defer { name } } { products { id ...SkuWithName @defer } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})
	})

}
