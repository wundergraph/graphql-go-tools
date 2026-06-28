package engine

import (
	"testing"
	"time"

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
						reportUnused: false,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{user {name}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black"}}}`,
								latency:    40 * time.Millisecond,
							},
							`{"query":"{user {__internal_typename: __typename}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__internal_typename":"User"}}}`,
							},
							`{"query":"{user {title}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"title":"Sabbat"}}}`,
								latency:    60 * time.Millisecond,
							},
							`{"query":"{user {id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1"}}}`,
								latency:    20 * time.Millisecond,
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
							`{"query":"{user {name info {__internal_typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"name":"Black","info":{"__internal_typename":"Info"}}}}`,
							},
							`{"query":"{user {info {__internal_typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"__internal_typename":"Info"}}}}`,
								latency:    80 * time.Millisecond,
							},
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
								latency:    10 * time.Millisecond,
							},
							`{"query":"{user {info {phone}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"phone":"123"}}}}`,
								latency:    20 * time.Millisecond,
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
						reportUnused: false,
						reportUsed:   false,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"{user {id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"id":"1"}}}`,
							},
							`{"query":"{user {__internal_typename: __typename __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__internal_typename":"User","__typename":"User","id":"1"}}}`,
							},
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
								latency:    20 * time.Millisecond,
							},
							`{"query":"{user {info {__internal_typename: __typename}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"__internal_typename":"Info"}}}}`,
								latency:    60 * time.Millisecond,
							},
							`{"query":"{user {__typename __internal_id: id __internal_1_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__typename":"User","__internal_id":"1","__internal_1_id":"1"}}}`,
							},
							`{"query":"{user {info {__internal_typename: __typename} __typename id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"__internal_typename":"Info"},"__typename":"User","id":"1"}}}`,
							},
							`{"query":"{user {__internal_typename: __typename __typename __internal_id: id}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"__internal_typename":"User","__typename":"User","__internal_id":"1"}}}`,
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
						reportUnused: false,
						reportUsed:   false,
						expectedHost: "second",
						expectedPath: "/",
						responses: map[string]sendResponse{
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black"}]}}`,
								latency:    20 * time.Millisecond,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","title":"Sabbat"}]}}`,
								latency:    40 * time.Millisecond,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename name title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","name":"Black","title":"Sabbat"}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {phone} title}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","info":{"phone":"123"},"title":"Sabbat"}]}}`,
							},
							`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename info {phone}}}}","variables":{"representations":[{"__typename":"User","id":"1"}]}}`: {
								statusCode: 200,
								body:       `{"data":{"_entities":[{"__typename":"User","info":{"phone":"123"}}]}}`,
								latency:    100 * time.Millisecond,
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]}],"hasNext":true}
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
				expectedResponse: `{"data":{"user":{"name":"Black"}},"pending":[{"id":"1","path":["user"],"label":"outer"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"],"label":"inner"}],"hasNext":true}
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
				expectedResponses: []string{
					`{"data":{"user":{}},"pending":[{"id":"1","path":["user"],"label":"a"},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
					`{"data":{"user":{}},"pending":[{"id":"1","path":["user"],"label":"a"},{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
				},
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
				expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]}],"hasNext":true}
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
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
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

			t.Run("merged/discarded defer parent", func(t *testing.T) {
				t.Run("defer nested object fields inside discarded parent defer", runWithoutError(ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							OperationName: "DeferUserTitle",
							Query: `
							query DeferUserTitle {
								user {
									info {
										email
									}
									... @defer {
										info {
											... @defer {
												phone
											}
										}
									}
								}
							}`,
						}
					},
					dataSources: tc.dataSources,
					expectedResponse: `{"data":{"user":{"info":{"email":"black@sabbat"}}},"pending":[{"id":"2","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":false}
`,
				}, withStreamingResponse()))

				t.Run("defer nested object fields inside merged parent defer", runWithoutError(ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							OperationName: "DeferUserTitle",
							Query: `
							query DeferUserTitle {
								user {
									... @defer {
										info {
											email
										}
									}
									... @defer {
										info {
											... @defer {
												phone
											}
										}
									}
								}
							}`,
						}
					},
					dataSources: tc.dataSources,
					expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"info":{"email":"black@sabbat"}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"3","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"3"}],"completed":[{"id":"3"}],"hasNext":false}
`,
				}, withStreamingResponse()))
			})

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
				// This test incremental order depends on latency of responses,
				// which gives us predictable order for the parallel fetches
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"user":{}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]},{"id":"3","path":["user"]},{"id":"4","path":["user"]},{"id":"5","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"3"}],"completed":[{"id":"3"}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"4"}],"completed":[{"id":"4"}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"id":"5"}],"completed":[{"id":"5"}],"pending":[{"id":"6","path":["user","info"]},{"id":"7","path":["user","info"]}],"hasNext":true}
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"user":{}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"2"}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Black"},"id":"3"}],"completed":[{"id":"3"}],"pending":[{"id":"4","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"title":"Sabbat"},"id":"4"}],"completed":[{"id":"4"}],"pending":[{"id":"5","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"info":{}},"id":"5"}],"completed":[{"id":"5"}],"pending":[{"id":"6","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"id":"6"}],"completed":[{"id":"6"}],"pending":[{"id":"7","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"7"}],"completed":[{"id":"7"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})
	}

	t.Run("defer parent ids across merged and discarded scopes", func(t *testing.T) {
		definition := `
			type Query {
				user: User!
			}
			type User {
				info: Info!
			}
			type Info {
				address: String!
				email: String!
				phone: String!
				nestedInfo: NestedInfo!
			}
			type NestedInfo {
				a: String!
				b: String!
				c: String!
				d: String!
			}
		`

		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		dataSources := []plan.DataSource{
			mustGraphqlDataSourceConfiguration(t,
				"id-1",
				mustFactory(t,
					testConditionalNetHttpClient(t, conditionalTestCase{
						reportUnused: false,
						expectedHost: "first",
						expectedPath: "/",
						responses: map[string]sendResponse{
							// initial response (non-deferred address)
							`{"query":"{user {info {address}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"address":"Berlin"}}}}`,
							},
							// defer id 1 (root): nestedInfo { a }
							`{"query":"{user {info {nestedInfo {a}}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"nestedInfo":{"a":"A"}}}}}`,
								latency:    10 * time.Millisecond,
							},
							// defer id 2 (parent 1): nestedInfo { b }
							`{"query":"{user {info {nestedInfo {b}}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"nestedInfo":{"b":"B"}}}}}`,
								latency:    20 * time.Millisecond,
							},
							// defer id 3 (parent 2): nestedInfo { c } and phone
							`{"query":"{user {info {nestedInfo {c} phone}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"nestedInfo":{"c":"C"},"phone":"123"}}}}`,
								latency:    30 * time.Millisecond,
							},
							// defer id 4 (parent 1): email
							`{"query":"{user {info {email}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"email":"black@sabbat"}}}}`,
								latency:    80 * time.Millisecond,
							},
							// defer id 5 (parent 4): nestedInfo { d }
							`{"query":"{user {info {nestedInfo {d}}}}"}`: {
								statusCode: 200,
								body:       `{"data":{"user":{"info":{"nestedInfo":{"d":"D"}}}}}`,
								latency:    90 * time.Millisecond,
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
							FieldNames: []string{"info"},
						},
						{
							TypeName:   "Info",
							FieldNames: []string{"address", "email", "phone", "nestedInfo"},
						},
						{
							TypeName:   "NestedInfo",
							FieldNames: []string{"a", "b", "c", "d"},
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

		t.Run("complex nested and sibling defers", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					OperationName: "DeferUserTitle",
					Query: `
					query DeferUserTitle {
						user {
							info {
								address
							}
						}
						... @defer {
							user {
								info {
									nestedInfo {
										a
									}
								}
								... @defer {
									info {
										nestedInfo {
											b
										}
										... @defer {
											nestedInfo {
												c
											}
											phone
										}
									}
								}
								... @defer {
									info {
										email
										... @defer {
											nestedInfo {
												d
											}
										}
									}
								}
							}
						}
					}`,
				}
			},
			dataSources: dataSources,
			// address is non-deferred (initial). The duplicated user/info/nestedInfo
			// containers merge, relocating every deferred leaf under one nestedInfo,
			// yet each defer id survives via its own leaf with a correct parent chain:
			// 1(root) -> 2 -> 3 and 1 -> 4 -> 5. The parent chains let the resolver
			// reach the deeply-merged fields (e.g. defer 2/3/5 traverse into the
			// defer-1 nestedInfo object).
			expectedResponse: `{"data":{"user":{"info":{"address":"Berlin"}}},"pending":[{"id":"1","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"nestedInfo":{"a":"A"}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user","info","nestedInfo"]},{"id":"4","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"b":"B"},"id":"2"}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["user","info"]}],"hasNext":true}
{"incremental":[{"data":{"phone":"123"},"id":"3"},{"data":{"c":"C"},"id":"3","subPath":["nestedInfo"]}],"completed":[{"id":"3"}],"hasNext":true}
{"incremental":[{"data":{"email":"black@sabbat"},"id":"4"}],"completed":[{"id":"4"}],"pending":[{"id":"5","path":["user","info","nestedInfo"]}],"hasNext":true}
{"incremental":[{"data":{"d":"D"},"id":"5"}],"completed":[{"id":"5"}],"hasNext":false}
`,
		}, withStreamingResponse()))
	})

	t.Run("defer across three federated subgraphs - article reviews authors", func(t *testing.T) {
		// Client-facing supergraph.
		definition := `
			type Query { article: Article }
			type Article { id: ID! title: String! reviews: [Review!]! }
			type Review { id: ID! author: Author! }
			type Author { id: ID! displayName: String! }
		`

		schema, err := graphql.NewSchemaFromString(definition)
		require.NoError(t, err)

		// Subgraph 1: owns Query.article and Article.{id,title}.
		articleSDL := `
			type Query { article: Article }
			type Article @key(fields: "id") { id: ID! title: String! }
		`
		articleDS := mustGraphqlDataSourceConfiguration(t,
			"id-1",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: false,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"{article {id __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"article":{"id":"a1","__typename":"Article"}}}`,
					},
					`{"query":"{article {title}}"}`: {
						statusCode: 200,
						body:       `{"data":{"article":{"title":"GraphQL Federation"}}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Query", FieldNames: []string{"article"}},
					{TypeName: "Article", FieldNames: []string{"id", "title"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "Article", SelectionSet: "id"},
					},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{URL: "https://first/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t,
					&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: articleSDL},
					articleSDL,
				),
			}),
		)

		// Subgraph 2: extends Article with reviews, owns Review.{id,author}, Author stub.
		reviewsSDL := `
			type Article @key(fields: "id") { id: ID! reviews: [Review!]! }
			type Review @key(fields: "id") { id: ID! author: Author! }
			type Author @key(fields: "id") { id: ID! }
		`
		reviewsDS := mustGraphqlDataSourceConfiguration(t,
			"id-2",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: false,
				expectedHost: "second",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {__typename reviews {id __typename}}}}","variables":{"representations":[{"__typename":"Article","id":"a1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Article","reviews":[{"id":"r1","__typename":"Review"},{"id":"r2","__typename":"Review"}]}]}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {__typename reviews {author {__typename id __internal_id: id}}}}}","variables":{"representations":[{"__typename":"Article","id":"a1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Article","reviews":[{"author":{"__typename":"Author","id":"u1","__internal_id":"u1"}},{"author":{"__typename":"Author","id":"u2","__internal_id":"u2"}}]}]}}`,
						// delay the author defer chain so the title defer (id 1)
						// always completes first, giving deterministic frame order.
						latency: 60 * time.Millisecond,
					},
					// nested case: the whole reviews subtree is deferred, so reviews
					// (and the nested author key) are fetched in the defer.
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Article {__typename reviews {id author {__typename id __internal_id: id}}}}}","variables":{"representations":[{"__typename":"Article","id":"a1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Article","reviews":[{"id":"r1","author":{"__typename":"Author","id":"u1","__internal_id":"u1"}},{"id":"r2","author":{"__typename":"Author","id":"u2","__internal_id":"u2"}}]}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Article", FieldNames: []string{"id", "reviews"}},
					{TypeName: "Review", FieldNames: []string{"id", "author"}},
					{TypeName: "Author", FieldNames: []string{"id"}},
				},
				ChildNodes: []plan.TypeField{
					{TypeName: "Review", FieldNames: []string{"id", "author"}},
					{TypeName: "Author", FieldNames: []string{"id"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "Article", SelectionSet: "id"},
						{TypeName: "Review", SelectionSet: "id"},
						{TypeName: "Author", SelectionSet: "id"},
					},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{URL: "https://second/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t,
					&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: reviewsSDL},
					reviewsSDL,
				),
			}),
		)

		// Subgraph 3: extends Author with displayName.
		authorsSDL := `
			type Author @key(fields: "id") { id: ID! displayName: String! }
		`
		authorsDS := mustGraphqlDataSourceConfiguration(t,
			"id-3",
			mustFactory(t, testConditionalNetHttpClient(t, conditionalTestCase{
				reportUnused: false,
				expectedHost: "third",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Author {__typename displayName}}}","variables":{"representations":[{"__typename":"Author","id":"u1"},{"__typename":"Author","id":"u2"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"Author","displayName":"Alice"},{"__typename":"Author","displayName":"Bob"}]}}`,
					},
				},
			})),
			&plan.DataSourceMetadata{
				RootNodes: []plan.TypeField{
					{TypeName: "Author", FieldNames: []string{"id", "displayName"}},
				},
				FederationMetaData: plan.FederationMetaData{
					Keys: plan.FederationFieldConfigurations{
						{TypeName: "Author", SelectionSet: "id"},
					},
				},
			},
			mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{URL: "https://third/", Method: "POST"},
				SchemaConfiguration: mustSchemaConfig(t,
					&graphql_datasource.FederationConfiguration{Enabled: true, ServiceSDL: authorsSDL},
					authorsSDL,
				),
			}),
		)

		dataSources := []plan.DataSource{articleDS, reviewsDS, authorsDS}

		t.Run("defer title and nested author across subgraphs", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ article { id ... @defer { __typename title} reviews{ id ... @defer { __typename author{ __typename id displayName } } } } }`,
				}
			},
			dataSources: dataSources,
			// article.__typename and each review's __typename are eager (their
			// objects are materialized in the initial response), so they appear in
			// the initial data and a literal __typename is available for the entity
			// jumps. title (defer 1) and author (defer 2, where author.__typename is
			// legitimately deferred because author is materialized in the defer) are
			// delivered incrementally.
			expectedResponse: `{"data":{"article":{"id":"a1","__typename":"Article","reviews":[{"id":"r1","__typename":"Review"},{"id":"r2","__typename":"Review"}]}},"pending":[{"id":"1","path":["article"]},{"id":"2","path":["article","reviews"]}],"hasNext":true}
{"incremental":[{"data":{"title":"GraphQL Federation"},"id":"1"}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"author":{"__typename":"Author","id":"u1","displayName":"Alice"}},"id":"2","subPath":[0]},{"data":{"author":{"__typename":"Author","id":"u2","displayName":"Bob"}},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
		}, withStreamingResponse()))

		t.Run("typename inside a deferred entity subtree stays deferred", runWithoutError(ExecutionEngineTestCase{
			schema: schema,
			operation: func(t *testing.T) graphql.Request {
				return graphql.Request{
					Query: `{ article { id ... @defer { reviews { id author { __typename id displayName } } } } }`,
				}
			},
			dataSources: dataSources,
			// reviews (and the nested author) are materialized inside the defer,
			// so author.__typename legitimately stays in the defer scope and is
			// delivered in the incremental frame, never eagerly.
			expectedResponse: `{"data":{"article":{"id":"a1"}},"pending":[{"id":"1","path":["article"]}],"hasNext":true}
{"incremental":[{"data":{"reviews":[{"id":"r1","author":{"__typename":"Author","id":"u1","displayName":"Alice"}},{"id":"r2","author":{"__typename":"Author","id":"u2","displayName":"Bob"}}]},"id":"1"}],"completed":[{"id":"1"}],"hasNext":false}
`,
		}, withStreamingResponse()))
	})

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
				reportUnused: false,
				reportUsed:   false,
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
					`{"query":"{user {__internal_typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"__internal_typename":"User"}}}`,
					},
					`{"query":"{user {name __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice","__typename":"User","id":"1"}}}`,
					},
					`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {__typename account {type}}}}","variables":{"representations":[{"__typename":"User","billing":{"plan":"pro"},"settings":{"region":"us-east"},"id":"1"}]}}`: {
						statusCode: 200,
						body:       `{"data":{"_entities":[{"__typename":"User","account":{"type":"premium"}}]}}`,
						latency:    20 * time.Millisecond,
					},
					`{"query":"{user {__internal_name: name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"__internal_name":"Alice"}}}`,
					},
					`{"query":"{user {name account {type} __internal_name: name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"name":"Alice","account":{"type":"premium"},"__internal_name":"Alice"}}}`,
					},
					`{"query":"{user {__internal_typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"user":{"__internal_typename":"User","__typename":"User","id":"1"}}}`,
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
				reportUnused: false,
				reportUsed:   false,
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
						latency:    50 * time.Millisecond,
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
				reportUnused: false,
				reportUsed:   false,
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
			expectedResponse: `{"data":{"user":{"name":"Alice"}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]}],"hasNext":true}
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
			expectedResponse: `{"data":{"user":{}},"pending":[{"id":"1","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Alice","billing":{"plan":"pro"}},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["user"]}],"hasNext":true}
{"incremental":[{"data":{"account":{"type":"premium"}},"id":"2"}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["user"]}],"hasNext":true}
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

		// makeDataSources builds the two product subgraph stubs with configurable
		// response latencies so order-dependent defer tests produce deterministic
		// frame ordering. The price defer group performs two roundtrips (key fetch
		// on id-1, then entity fetch on id-2), so its total latency is
		// keyLatencyMS + entityLatencyMS.
		makeDataSources := func(nameLatency, nameWithErrorLatency, keyLatency, entityLatency time.Duration) []plan.DataSource {
			return []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id-1",
					mustFactory(t,
						testConditionalNetHttpClient(t, conditionalTestCase{
							reportUnused: false,
							expectedHost: "first",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"{product {__internal_typename: __typename}}"}`: {
									statusCode: 200,
									body:       `{"data":{"product":{"__internal_typename":"Product"}}}`,
								},
								`{"query":"{product {__internal_typename: __typename __typename id}}"}`: {
									statusCode: 200,
									body:       `{"data":{"product":{"__internal_typename":"Product","__typename":"Product","id":"1"}}}`,
									latency:    keyLatency,
								},
								`{"query":"{product {name}}"}`: {
									statusCode: 200,
									body:       `{"data":{"product":{"name":null}}}`,
									latency:    nameLatency,
								},
								`{"query":"{product {nameWithError}}"}`: {
									statusCode: 200,
									body:       `{"data":{"product":{"nameWithError":null}},"errors":[{"message":"upstream name error","path":["product","nameWithError"]}]}`,
									latency:    nameWithErrorLatency,
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
							reportUnused: false,
							expectedHost: "second",
							expectedPath: "/",
							responses: map[string]sendResponse{
								`{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {__typename price}}}","variables":{"representations":[{"__typename":"Product","id":"1"}]}}`: {
									statusCode: 200,
									body:       `{"data":{"_entities":[{"__typename":"Product","price":null}]}}`,
									latency:    entityLatency,
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
		}

		dataSources := makeDataSources(0, 0, 0, 0)

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
			// name must complete first: name responds in 1ms, the price chain
			// (key fetch + entity fetch) takes ~10ms.
			dataSources: makeDataSources(10*time.Millisecond, 0, 50*time.Millisecond, 50*time.Millisecond),
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
			// price must complete first: the price chain (key + entity) takes
			// ~2ms, name responds after 12ms.
			dataSources: makeDataSources(50*time.Millisecond, 0, 10*time.Millisecond, 10*time.Millisecond),
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
			// nameWithError must complete first: it responds in 1ms, the price
			// chain (key + entity) takes ~10ms.
			dataSources: makeDataSources(0, 10*time.Millisecond, 50*time.Millisecond, 50*time.Millisecond),
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
				reportUnused: false,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"{items {__internal_typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"__internal_typename":"Item","__typename":"Item","id":"1"},{"__internal_typename":"Item","__typename":"Item","id":"2"}]}}`,
					},
					`{"query":"{items {name}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"name":"ItemOne"},{"name":"ItemTwo"}]}}`,
					},
					`{"query":"{items {__internal_typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"__internal_typename":"Item"},{"__internal_typename":"Item"}]}}`,
					},
					`{"query":"{items {subItems {__internal_typename: __typename __typename id}}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"subItems":[{"__internal_typename":"SubItem","__typename":"SubItem","id":"s1"},{"__internal_typename":"SubItem","__typename":"SubItem","id":"s2"}]},{"subItems":[{"__internal_typename":"SubItem","__typename":"SubItem","id":"s3"}]}]}}`,
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
					`{"query":"{items {__internal_typename: __typename __typename __internal_id: id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"items":[{"__internal_typename":"Item","__typename":"Item","__internal_id":"1"},{"__internal_typename":"Item","__typename":"Item","__internal_id":"2"}]}}`,
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
				reportUnused: false,
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
				reportUnused: false,
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["items"]}],"hasNext":true}
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
				expectedResponses: []string{
					`{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
					`{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
				},
			}, withStreamingResponse()))

			t.Run("defer id in parallel with title (cross-subgraph)", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { ... @defer { id } ... @defer { title } } }`}
				},
				dataSources: dataSources,
				expectedResponses: []string{
					`{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
					`{"data":{"items":[{},{}]},"pending":[{"id":"1","path":["items"]},{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"id":"1"},"id":"1","subPath":[0]},{"data":{"id":"2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
				},
			}, withStreamingResponse()))

			t.Run("parallel defers on subItems id and description", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ items { id ... @defer { subItems { id } } ... @defer { subItems { description } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{"items":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"1","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"subItems":[{"id":"s1"},{"id":"s2"}]},{"subItems":[{"id":"s3"}]}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
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
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"name":"ItemOne"},"id":"2","subPath":[0]},{"data":{"name":"ItemTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("outer defer items, inner defer title (cross-subgraph)", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { title } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne"},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("outer defer items with subItems, inner defer description", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id subItems { id ... @defer { description } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1","subItems":[{"id":"s1"},{"id":"s2"}]},{"id":"2","subItems":[{"id":"s3"}]}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"2","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"2","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"2","subPath":[1,"subItems",0]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("three-level defer: query to items to subItems", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { subItems { id ... @defer { description } } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"description":"Desc1"},"id":"3","subPath":[0,"subItems",0]},{"data":{"description":"Desc2"},"id":"3","subPath":[0,"subItems",1]},{"data":{"description":"Desc3"},"id":"3","subPath":[1,"subItems",0]}],"completed":[{"id":"3"}],"hasNext":false}
`,
			}, withStreamingResponse()))

			t.Run("three-level defer with cross-subgraph at middle level", runWithoutError(ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{ ... @defer { items { id ... @defer { title subItems { id ... @defer { description } } } } } }`}
				},
				dataSources: dataSources,
				expectedResponse: `{"data":{},"pending":[{"id":"1","path":[]}],"hasNext":true}
{"incremental":[{"data":{"items":[{"id":"1"},{"id":"2"}]},"id":"1"}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["items"]}],"hasNext":true}
{"incremental":[{"data":{"title":"TitleOne","subItems":[{"id":"s1"},{"id":"s2"}]},"id":"2","subPath":[0]},{"data":{"title":"TitleTwo","subItems":[{"id":"s3"}]},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"pending":[{"id":"3","path":["items"]}],"hasNext":true}
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
				reportUnused: false,
				expectedHost: "first",
				expectedPath: "/",
				responses: map[string]sendResponse{
					`{"query":"{products {__internal_typename: __typename __typename id}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"__internal_typename":"Product","__typename":"Product","id":"1"},{"__internal_typename":"Product","__typename":"Product","id":"2"}]}}`,
					},
					`{"query":"{products {__internal_typename: __typename}}"}`: {
						statusCode: 200,
						body:       `{"data":{"products":[{"__internal_typename":"Product"},{"__internal_typename":"Product"}]}}`,
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
				reportUnused: false,
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
				expectedResponses: []string{
					`{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
					`{"data":{"products":[{},{}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
				},
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
				expectedResponses: []string{
					`{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
					`{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]},{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"hasNext":false}
`,
				},
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
				expectedResponse: `{"data":{"products":[{"id":"1"},{"id":"2"}]},"pending":[{"id":"1","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"sku":"sku-1"},"id":"1","subPath":[0]},{"data":{"sku":"sku-2"},"id":"1","subPath":[1]}],"completed":[{"id":"1"}],"pending":[{"id":"2","path":["products"]}],"hasNext":true}
{"incremental":[{"data":{"name":"Product One"},"id":"2","subPath":[0]},{"data":{"name":"Product Two"},"id":"2","subPath":[1]}],"completed":[{"id":"2"}],"hasNext":false}
`,
			}, withStreamingResponse()))
		})
	})

}
