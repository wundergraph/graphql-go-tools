package execution

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"go.uber.org/zap"
	"math/rand"
	"reflect"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func TestPlanner_Plan(t *testing.T) {
	run := func(definition string, operation string, resolverDefinitions ResolverDefinitions, want Node) func(t *testing.T) {
		return func(t *testing.T) {
			def := unsafeparser.ParseGraphqlDocumentString(definition)
			op := unsafeparser.ParseGraphqlDocumentString(operation)

			var report operationreport.Report
			normalizer := astnormalization.NewNormalizer(true)
			normalizer.NormalizeOperation(&op, &def, &report)
			if report.HasErrors() {
				t.Error(report)
			}

			planner := NewPlanner(resolverDefinitions)
			got := planner.Plan(&op, &def, &report)
			if report.HasErrors() {
				t.Error(report)
			}

			if !reflect.DeepEqual(want, got) {
				fmt.Println(deep.Equal(want, got))
				diffview.NewGoland().DiffViewAny("diff", want, got)
				t.Errorf("want:\n%s\ngot:\n%s\n", spew.Sdump(want), spew.Sdump(got))
			}
		}
	}

	t.Run("GraphQLDataSource", run(withBaseSchema(GraphQLDataSourceSchema), `
				query GraphQLQuery($code: String!) {
					country(code: $code) {
						code
						name
						aliased: native
					}
				}
`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("country"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("countries.trevorblades.com"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&StaticVariableArgument{
										Name:  []byte("query"),
										Value: []byte("query o($code: String!){country(code: $code){code name native}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("code"),
										VariableName: []byte("code"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							BufferName: "country",
						},
						Fields: []Field{
							{
								Name:        []byte("country"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"country"},
									Fields: []Field{
										{
											Name: []byte("code"),
											Value: &Value{
												Path:       []string{"code"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("aliased"),
											Value: &Value{
												Path:       []string{"native"},
												QuoteValue: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("GraphQLDataSource mutation", run(withBaseSchema(GraphQLDataSourceSchema), `
				mutation LikePost($id: ID!) {
					likePost(id: $id) {
						id
						likes
					}
				}
`,
		ResolverDefinitions{
			{
				TypeName:  literal.MUTATION,
				FieldName: []byte("likePost"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeMutation,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("fakebook.com"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&StaticVariableArgument{
										Name:  []byte("query"),
										Value: []byte("mutation o($id: ID!){likePost(id: $id){id likes}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							BufferName: "likePost",
						},
						Fields: []Field{
							{
								Name:        []byte("likePost"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"likePost"},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("likes"),
											Value: &Value{
												Path:       []string{"likes"},
												QuoteValue: false,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("HTTPJSONDataSource", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query RESTQuery($id: Int!){
						httpBinGet {
							header {
								Accept
								Host
								acceptEncoding
							}
						}
						post(id: $id) {
							id
							comments {
								id
							}
						}
					}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("httpBinGet"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("post"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
			{
				TypeName:  []byte("JSONPlaceholderPost"),
				FieldName: []byte("comments"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &ParallelFetch{
							Fetches: []Fetch{
								&SingleFetch{
									Source: &DataSourceInvocation{
										DataSource: &HttpJsonDataSource{},
										Args: []Argument{
											&StaticVariableArgument{
												Name:  []byte("host"),
												Value: []byte("httpbin.org"),
											},
											&StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("/get"),
											},
											&StaticVariableArgument{
												Name:  []byte("method"),
												Value: []byte("GET"),
											},
										},
									},
									BufferName: "httpBinGet",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []Argument{
											&StaticVariableArgument{
												Name:  []byte("host"),
												Value: []byte("jsonplaceholder.typicode.com"),
											},
											&StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("/posts/{{ .arguments.id }}"),
											},
											&StaticVariableArgument{
												Name:  []byte("method"),
												Value: []byte("GET"),
											},
											&ContextVariableArgument{
												Name:         []byte(".arguments.id"),
												VariableName: []byte("id"),
											},
										},
										DataSource: &HttpJsonDataSource{},
									},
									BufferName: "post",
								},
							},
						},
						Fields: []Field{
							{
								Name:        []byte("httpBinGet"),
								HasResolver: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("header"),
											Value: &Object{
												Path: []string{"headers"},
												Fields: []Field{
													{
														Name: []byte("Accept"),
														Value: &Value{
															Path:       []string{"Accept"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("Host"),
														Value: &Value{
															Path:       []string{"Host"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("acceptEncoding"),
														Value: &Value{
															Path:       []string{"Accept-Encoding"},
															QuoteValue: true,
														},
													},
												},
											},
										},
									},
								},
							},
							{
								Name:        []byte("post"),
								HasResolver: true,
								Value: &Object{
									Fetch: &SingleFetch{
										Source: &DataSourceInvocation{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  []byte("host"),
													Value: []byte("jsonplaceholder.typicode.com"),
												},
												&StaticVariableArgument{
													Name:  []byte("url"),
													Value: []byte("/comments?postId={{ .postId }}"),
												},
												&ObjectVariableArgument{
													Name: []byte("postId"),
													Path: []string{"id"},
												},
												&StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
											},
											DataSource: &HttpJsonDataSource{},
										},
										BufferName: "comments",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: false,
											},
										},
										{
											Name:        []byte("comments"),
											HasResolver: true,
											Value: &List{
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("id"),
															Value: &Value{
																Path:       []string{"id"},
																QuoteValue: false,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("HTTPJSONDataSource withBody", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query WithBody($input: WithBodyInput) {
						withBody(input: $input)
					}
					`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("withBody"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "withBody",
							Source: &DataSourceInvocation{
								DataSource: &HttpJsonDataSource{},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&StaticVariableArgument{
										Name:  []byte("body"),
										Value: []byte("{\\\"key\\\":\\\"{{ .arguments.input.foo }}\\\"}"),
									},
									&ContextVariableArgument{
										Name:         []byte(".arguments.input"),
										VariableName: []byte("input"),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:        []byte("withBody"),
								HasResolver: true,
								Value: &Value{
									QuoteValue: true,
								},
							},
						},
					},
				},
			},
		}))
	t.Run("HTTPJSONDataSource withPath", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query WithPath {
						withPath
					}
					`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("withPath"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "withPath",
							Source: &DataSourceInvocation{
								DataSource: &HttpJsonDataSource{},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name: []byte("withPath"),
								HasResolver: true,
								Value: &Value{
									Path:       []string{"subObject","withPath"},
									QuoteValue: true,
								},
							},
						},
					},
				},
			},
		}))
	t.Run("HTTPJSONDataSource withHeaders", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query WithHeader {
						withHeaders
					}
					`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("withHeaders"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "withHeaders",
							Source: &DataSourceInvocation{
								DataSource: &HttpJsonDataSource{},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&ListArgument{
										Name: []byte("headers"),
										Arguments: []Argument{
											&StaticVariableArgument{
												Name:  []byte("Authorization"),
												Value: []byte("123"),
											},
											&StaticVariableArgument{
												Name:  []byte("Accept-Encoding"),
												Value: []byte("application/json"),
											},
										},
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:        []byte("withHeaders"),
								HasResolver: true,
								Value: &Value{
									QuoteValue: true,
								},
							},
						},
					},
				},
			},
		}))
	t.Run("StaticDataSource", run(withBaseSchema(staticDataSourceSchema), `
					{
						hello
						nullableInt
						foo {
							bar
						}
					}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("hello"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &StaticDataSourcePlanner{}
				},
			},
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("nullableInt"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &StaticDataSourcePlanner{}
				},
			},
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("foo"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &StaticDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &ParallelFetch{
							Fetches: []Fetch{
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []Argument{
											&StaticVariableArgument{
												Value: []byte("World!"),
											},
										},
										DataSource: &StaticDataSource{},
									},
									BufferName: "hello",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []Argument{
											&StaticVariableArgument{
												Value: []byte("null"),
											},
										},
										DataSource: &StaticDataSource{},
									},
									BufferName: "nullableInt",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []Argument{
											&StaticVariableArgument{
												Value: []byte("{\"bar\":\"baz\"}"),
											},
										},
										DataSource: &StaticDataSource{},
									},
									BufferName: "foo",
								},
							},
						},
						Fields: []Field{
							{
								Name:        []byte("hello"),
								HasResolver: true,
								Value: &Value{
									QuoteValue: true,
								},
							},
							{
								Name:        []byte("nullableInt"),
								HasResolver: true,
								Value: &Value{
									QuoteValue: false,
								},
							},
							{
								Name:        []byte("foo"),
								HasResolver: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("bar"),
											Value: &Value{
												Path:       []string{"bar"},
												QuoteValue: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("introspection type query", run(withBaseSchema(complexSchema), `
				query TypeQuery($name: String! = "User") {
					__type(name: $name) {
						name
						fields {
							name
							type {
								name
							}
						}
					}
				}
`, ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: literal.UNDERSCORETYPE,
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &TypeDataSourcePlanner{}
			},
		},
	}, &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&ContextVariableArgument{
									Name:         []byte("name"),
									VariableName: []byte("name"),
								},
							},
							DataSource: &TypeDataSource{},
						},
						BufferName: "__type",
					},
					Fields: []Field{
						{
							Name:        []byte("__type"),
							HasResolver: true,
							Value: &Object{
								Path: []string{"__type"},
								Fields: []Field{
									{
										Name: []byte("name"),
										Value: &Value{
											Path:       []string{"name"},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("fields"),
										Value: &List{
											Path: []string{"fields"},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("type"),
														Value: &Object{
															Path: []string{"type"},
															Fields: []Field{
																{
																	Name: []byte("name"),
																	Value: &Value{
																		Path:       []string{"name"},
																		QuoteValue: true,
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}))
	t.Run("graphql resolver", run(withBaseSchema(complexSchema), `
			query UserQuery($id: String!) {
				user(id: $id) {
					id
					name
					birthday
				}
			}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("user"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							BufferName: "user",
						},
						Fields: []Field{
							{
								Name:        []byte("user"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"user"},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												Path:       []string{"birthday"},
												QuoteValue: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))
	t.Run("rest resolver", run(withBaseSchema(complexSchema), `
				query UserQuery($id: String!) {
					restUser(id: $id) {
						id
						name
						birthday
					}
				}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("restUser"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:9001"),
									},
									&StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/user/{{ .arguments.id }}"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&ContextVariableArgument{
										Name:         []byte(".arguments.id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &HttpJsonDataSource{},
							},
							BufferName: "restUser",
						},
						Fields: []Field{
							{
								Name:        []byte("restUser"),
								HasResolver: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												Path:       []string{"birthday"},
												QuoteValue: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("graphql resolver with nested rest resolver", run(withBaseSchema(complexSchema), `
			query UserQuery($id: String!) {
				user(id: $id) {
					id
					name
					birthday
					friends {
						id
						name
						birthday
					}
				}
			}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("user"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
			{
				TypeName:  []byte("User"),
				FieldName: []byte("friends"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							BufferName: "user",
						},
						Fields: []Field{
							{
								Name:        []byte("user"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"user"},
									Fetch: &SingleFetch{
										Source: &DataSourceInvocation{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  literal.HOST,
													Value: []byte("localhost:9000"),
												},
												&StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/user/:id/friends"),
												},
												&ObjectVariableArgument{
													Name: []byte("id"),
													Path: []string{"id"},
												},
												&StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
											},
											DataSource: &HttpJsonDataSource{},
										},
										BufferName: "friends",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												Path:       []string{"birthday"},
												QuoteValue: true,
											},
										},
										{
											Name:        []byte("friends"),
											HasResolver: true,
											Value: &List{
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("id"),
															Value: &Value{
																Path:       []string{"id"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("birthday"),
															Value: &Value{
																Path:       []string{"birthday"},
																QuoteValue: true,
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))
	t.Run("nested rest and graphql resolver", run(withBaseSchema(complexSchema), `
			query UserQuery($id: String!) {
				user(id: $id) {
					id
					name
					friends {
						id
						name
						birthday
						pets {
							__typename
							nickname
							... on Dog {
								name
								woof
							}
							... on Cat {
								name
								meow
							}
						}
					}
					pets {
						__typename
						nickname
						... on Dog {
							name
							woof
						}
						... on Cat {
							name
							meow
						}
					}
					birthday
				}
			}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("user"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
			{
				TypeName:  []byte("User"),
				FieldName: []byte("friends"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
			{
				TypeName:  []byte("User"),
				FieldName: []byte("pets"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{},
							},
							BufferName: "user",
						},
						Fields: []Field{
							{
								Name:        []byte("user"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"user"},
									Fetch: &ParallelFetch{
										Fetches: []Fetch{
											&SingleFetch{
												Source: &DataSourceInvocation{
													Args: []Argument{
														&StaticVariableArgument{
															Name:  literal.HOST,
															Value: []byte("localhost:9000"),
														},
														&StaticVariableArgument{
															Name:  literal.URL,
															Value: []byte("/user/:id/friends"),
														},
														&ObjectVariableArgument{
															Name: []byte("id"),
															Path: []string{"id"},
														},
														&StaticVariableArgument{
															Name:  []byte("method"),
															Value: []byte("GET"),
														},
													},
													DataSource: &HttpJsonDataSource{},
												},
												BufferName: "friends",
											},
											&SingleFetch{
												Source: &DataSourceInvocation{
													Args: []Argument{
														&StaticVariableArgument{
															Name:  literal.HOST,
															Value: []byte("localhost:8002"),
														},
														&StaticVariableArgument{
															Name:  literal.URL,
															Value: []byte("/graphql"),
														},
														&StaticVariableArgument{
															Name:  literal.QUERY,
															Value: []byte("query o($userId: String!){userPets(userId: $userId){__typename nickname ... on Dog {name woof} ... on Cat {name meow}}}"),
														},
														&ObjectVariableArgument{
															Name: []byte("userId"),
															Path: []string{"id"},
														},
													},
													DataSource: &GraphQLDataSource{},
												},
												BufferName: "pets",
											},
										},
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												Path:       []string{"id"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name:        []byte("friends"),
											HasResolver: true,
											Value: &List{
												Value: &Object{
													Fetch: &SingleFetch{
														Source: &DataSourceInvocation{
															Args: []Argument{
																&StaticVariableArgument{
																	Name:  literal.HOST,
																	Value: []byte("localhost:8002"),
																},
																&StaticVariableArgument{
																	Name:  literal.URL,
																	Value: []byte("/graphql"),
																},
																&StaticVariableArgument{
																	Name:  literal.QUERY,
																	Value: []byte("query o($userId: String!){userPets(userId: $userId){__typename nickname ... on Dog {name woof} ... on Cat {name meow}}}"),
																},
																&ObjectVariableArgument{
																	Name: []byte("userId"),
																	Path: []string{"id"},
																},
															},
															DataSource: &GraphQLDataSource{},
														},
														BufferName: "pets",
													},
													Fields: []Field{
														{
															Name: []byte("id"),
															Value: &Value{
																Path:       []string{"id"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("birthday"),
															Value: &Value{
																Path:       []string{"birthday"},
																QuoteValue: true,
															},
														},
														{
															Name:        []byte("pets"),
															HasResolver: true,
															Value: &List{
																Path: []string{"userPets"},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("__typename"),
																			Value: &Value{
																				Path:       []string{"__typename"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("nickname"),
																			Value: &Value{
																				Path:       []string{"nickname"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					Path: []string{"__typename"},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("woof"),
																			Value: &Value{
																				Path:       []string{"woof"},
																				QuoteValue: true,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					Path: []string{"__typename"},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					Path: []string{"__typename"},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Cat"),
																				},
																			},
																		},
																		{
																			Name: []byte("meow"),
																			Value: &Value{
																				Path:       []string{"meow"},
																				QuoteValue: true,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					Path: []string{"__typename"},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Cat"),
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
										{
											Name:        []byte("pets"),
											HasResolver: true,
											Value: &List{
												Path: []string{"userPets"},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("__typename"),
															Value: &Value{
																Path:       []string{"__typename"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("nickname"),
															Value: &Value{
																Path:       []string{"nickname"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	Path: []string{"__typename"},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("woof"),
															Value: &Value{
																Path:       []string{"woof"},
																QuoteValue: true,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	Path: []string{"__typename"},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	Path: []string{"__typename"},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Cat"),
																},
															},
														},
														{
															Name: []byte("meow"),
															Value: &Value{
																Path:       []string{"meow"},
																QuoteValue: true,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	Path: []string{"__typename"},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Cat"),
																},
															},
														},
													},
												},
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												Path:       []string{"birthday"},
												QuoteValue: true,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))
	t.Run("introspection", run(withBaseSchema(complexSchema), `
			query IntrospectionQuery {
			  __schema {
				queryType {
				  name
				}
				mutationType {
				  name
				}
				subscriptionType {
				  name
				}
				types {
				  ...FullType
				}
				directives {
				  name
				  description
				  locations
				  args {
					...InputValue
				  }
				}
			  }
			}
			
			fragment FullType on __Type {
			  kind
			  name
			  description
			  fields(includeDeprecated: true) {
				name
				description
				args {
				  ...InputValue
				}
				type {
				  ...TypeRef
				}
				isDeprecated
				deprecationReason
			  }
			  inputFields {
				...InputValue
			  }
			  interfaces {
				...TypeRef
			  }
			  enumValues(includeDeprecated: true) {
				name
				description
				isDeprecated
				deprecationReason
			  }
			  possibleTypes {
				...TypeRef
			  }
			}
			
			fragment InputValue on __InputValue {
			  name
			  description
			  type {
				...TypeRef
			  }
			  defaultValue
			}
			
			fragment TypeRef on __Type {
			  kind
			  name
			  ofType {
				kind
				name
				ofType {
				  kind
				  name
				  ofType {
					kind
					name
					ofType {
					  kind
					  name
					  ofType {
						kind
						name
						ofType {
						  kind
						  name
						  ofType {
							kind
							name
						  }
						}
					  }
					}
				  }
				}
			  }
			}`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: literal.UNDERSCORESCHEMA,
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &SchemaDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &SchemaDataSource{},
							},
							BufferName: "__schema",
						},
						Fields: []Field{
							{
								Name:        []byte("__schema"),
								HasResolver: true,
								Value: &Object{
									Path: []string{"__schema"},
									Fields: []Field{
										{
											Name: []byte("queryType"),
											Value: &Object{
												Path: []string{"queryType"},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
												},
											},
										},
										{
											Name: []byte("mutationType"),
											Value: &Object{
												Path: []string{"mutationType"},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
												},
											},
										},
										{
											Name: []byte("subscriptionType"),
											Value: &Object{
												Path: []string{"subscriptionType"},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
												},
											},
										},
										{
											Name: []byte("types"),
											Value: &List{
												Path: []string{"types"},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("kind"),
															Value: &Value{
																Path:       []string{"kind"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("description"),
															Value: &Value{
																Path:       []string{"description"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("fields"),
															Value: &List{
																Path: []string{"fields"},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				Path:       []string{"description"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("args"),
																			Value: &List{
																				Path: []string{"args"},
																				Value: &Object{
																					Fields: []Field{
																						{
																							Name: []byte("name"),
																							Value: &Value{
																								Path:       []string{"name"},
																								QuoteValue: true,
																							},
																						},
																						{
																							Name: []byte("description"),
																							Value: &Value{
																								Path:       []string{"description"},
																								QuoteValue: true,
																							},
																						},
																						{
																							Name: []byte("type"),
																							Value: &Object{
																								Path:   []string{"type"},
																								Fields: kindNameDeepFields,
																							},
																						},
																						{
																							Name: []byte("defaultValue"),
																							Value: &Value{
																								Path:       []string{"defaultValue"},
																								QuoteValue: true,
																							},
																						},
																					},
																				},
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				Path:   []string{"type"},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("isDeprecated"),
																			Value: &Value{
																				Path:       []string{"isDeprecated"},
																				QuoteValue: false,
																			},
																		},
																		{
																			Name: []byte("deprecationReason"),
																			Value: &Value{
																				Path:       []string{"deprecationReason"},
																				QuoteValue: true,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("inputFields"),
															Value: &List{
																Path: []string{"inputFields"},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				Path:       []string{"description"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				Path:   []string{"type"},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("defaultValue"),
																			Value: &Value{
																				Path:       []string{"defaultValue"},
																				QuoteValue: true,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("interfaces"),
															Value: &List{
																Path: []string{"interfaces"},
																Value: &Object{
																	Fields: kindNameDeepFields,
																},
															},
														},
														{
															Name: []byte("enumValues"),
															Value: &List{
																Path: []string{"enumValues"},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				Path:       []string{"description"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("isDeprecated"),
																			Value: &Value{
																				Path:       []string{"isDeprecated"},
																				QuoteValue: false,
																			},
																		},
																		{
																			Name: []byte("deprecationReason"),
																			Value: &Value{
																				Path:       []string{"deprecationReason"},
																				QuoteValue: true,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("possibleTypes"),
															Value: &List{
																Path: []string{"possibleTypes"},
																Value: &Object{
																	Fields: kindNameDeepFields,
																},
															},
														},
													},
												},
											},
										},
										{
											Name: []byte("directives"),
											Value: &List{
												Path: []string{"directives"},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																Path:       []string{"name"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("description"),
															Value: &Value{
																Path:       []string{"description"},
																QuoteValue: true,
															},
														},
														{
															Name: []byte("locations"),
															Value: &List{
																Path: []string{"locations"},
																Value: &Value{
																	QuoteValue: true,
																},
															},
														},
														{
															Name: []byte("args"),
															Value: &List{
																Path: []string{"args"},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				Path:       []string{"name"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				Path:       []string{"description"},
																				QuoteValue: true,
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				Path:   []string{"type"},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("defaultValue"),
																			Value: &Value{
																				Path:       []string{"defaultValue"},
																				QuoteValue: true,
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))

	t.Run("http polling stream", run(withBaseSchema(HttpPollingStreamSchema), `
					subscription {
						stream {
							bar
							baz
						}
					}
`, []DataSourceDefinition{
		{
			TypeName:  literal.SUBSCRIPTION,
			FieldName: []byte("stream"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &HttpPollingStreamDataSourcePlanner{
					BaseDataSourcePlanner: BaseDataSourcePlanner{
						log: zap.NewNop(),
					},
				}
			},
		},
	}, &Object{
		operationType: ast.OperationTypeSubscription,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Name:  literal.HOST,
									Value: []byte("foo.bar.baz"),
								},
								&StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte("/bal"),
								},
							},
							DataSource: &HttpPollingStreamDataSource{
								log:   zap.NewNop(),
								delay: time.Second * 5,
							},
						},
						BufferName: "stream",
					},
					Fields: []Field{
						{
							Name:        []byte("stream"),
							HasResolver: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("bar"),
										Value: &Value{
											Path:       []string{"bar"},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("baz"),
										Value: &Value{
											Path:       []string{"baz"},
											QuoteValue: false,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}))
	t.Run("http polling stream inline delay", run(withBaseSchema(HttpPollingStreamSchemaInlineDelay), `
					subscription {
						stream {
							bar
							baz
						}
					}
`, []DataSourceDefinition{
		{
			TypeName:  literal.SUBSCRIPTION,
			FieldName: []byte("stream"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &HttpPollingStreamDataSourcePlanner{
					BaseDataSourcePlanner: BaseDataSourcePlanner{
						log: zap.NewNop(),
					},
				}
			},
		},
	}, &Object{
		operationType: ast.OperationTypeSubscription,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							Args: []Argument{
								&StaticVariableArgument{
									Name:  literal.HOST,
									Value: []byte("foo.bar.baz"),
								},
								&StaticVariableArgument{
									Name:  literal.URL,
									Value: []byte("/bal"),
								},
							},
							DataSource: &HttpPollingStreamDataSource{
								log:   zap.NewNop(),
								delay: time.Second * 3,
							},
						},
						BufferName: "stream",
					},
					Fields: []Field{
						{
							Name:        []byte("stream"),
							HasResolver: true,
							Value: &Object{
								Fields: []Field{
									{
										Name: []byte("bar"),
										Value: &Value{
											Path:       []string{"bar"},
											QuoteValue: true,
										},
									},
									{
										Name: []byte("baz"),
										Value: &Value{
											Path:       []string{"baz"},
											QuoteValue: false,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}))
	t.Run("list filter first N", run(withBaseSchema(ListFilterFirstNSchema), `
			query {
				foos {
					bar
				}
			}
		`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("foos"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &StaticDataSourcePlanner{}
				},
			},
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []Argument{
									&StaticVariableArgument{
										Value: []byte("[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]"),
									},
								},
								DataSource: &StaticDataSource{},
							},
							BufferName: "foos",
						},
						Fields: []Field{
							{
								Name:        []byte("foos"),
								HasResolver: true,
								Value: &List{
									Filter: &ListFilterFirstN{
										FirstN: 2,
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("bar"),
												Value: &Value{
													Path:       []string{"bar"},
													QuoteValue: true,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	))
}

func BenchmarkPlanner_Plan(b *testing.B) {
	def := unsafeparser.ParseGraphqlDocumentString(withBaseSchema(complexSchema))
	op := unsafeparser.ParseGraphqlDocumentString(`
			query UserQuery($id: String!) {
				user(id: $id) {
					id
					name
					friends {
						id
						name
						birthday
						pets {
							__typename
							nickname
							... on Dog {
								name
								woof
							}
							... on Cat {
								name
								meow
							}
						}
					}
					pets {
						__typename
						nickname
						... on Dog {
							name
							woof
						}
						... on Cat {
							name
							meow
						}
					}
					birthday
				}
			}`)

	resolverDefinitions := ResolverDefinitions{
		{
			TypeName:  literal.QUERY,
			FieldName: []byte("user"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
			},
		},
		{
			TypeName:  []byte("User"),
			FieldName: []byte("friends"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return &HttpJsonDataSourcePlanner{}
			},
		},
		{
			TypeName:  []byte("User"),
			FieldName: []byte("pets"),
			DataSourcePlannerFactory: func() DataSourcePlanner {
				return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{})
			},
		},
	}

	planner := NewPlanner(resolverDefinitions)
	var report operationreport.Report

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		planner.Plan(&op, &def, &report)
		if report.HasErrors() {
			b.Fatal(report)
		}
	}
}

const ListFilterFirstNSchema = `
directive @ListFilterFirstN(n: Int!) on FIELD_DEFINITION

schema {
	query: Query
}

type Query {
	foos: [Foo]
		@ListFilterFirstN(n: 2)
		@StaticDataSource(
            data: "[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]"
        )
}

type Foo {
	bar: String
}

`

const HttpPollingStreamSchema = `
directive @HttpPollingStreamDataSource (
    host: String!
    url: String!
    method: HTTP_METHOD = GET
    delaySeconds: Int = 5
    params: [Parameter]
) on FIELD_DEFINITION

schema {
	subscription: Subscription
}

type Subscription {
	stream: Foo
		@HttpPollingStreamDataSource(
			host: "foo.bar.baz"
			url: "/bal"
		)
}

type Foo {
	bar: String
	baz: Int
}
`

const HttpPollingStreamSchemaInlineDelay = `
directive @HttpPollingStreamDataSource (
    host: String!
    url: String!
    method: HTTP_METHOD = GET
    delaySeconds: Int = 5
    params: [Parameter]
) on FIELD_DEFINITION

schema {
	subscription: Subscription
}

type Subscription {
	stream: Foo
		@HttpPollingStreamDataSource(
			host: "foo.bar.baz"
			url: "/bal"
			delaySeconds: 3
		)
}

type Foo {
	bar: String
	baz: Int
}
`

const GraphQLDataSourceSchema = `
directive @GraphQLDataSource (
    host: String!
    url: String!
	method: HTTP_METHOD = POST
    params: [Parameter]
) on FIELD_DEFINITION

directive @mapTo(
	objectField: String!
) on FIELD_DEFINITION

enum HTTP_METHOD {
    GET
    POST
    UPDATE
    DELETE
}

input Parameter {
    name: String!
    sourceKind: PARAMETER_SOURCE!
    sourceName: String!
    variableType: String!
}

enum PARAMETER_SOURCE {
    CONTEXT_VARIABLE
    OBJECT_VARIABLE_ARGUMENT
    FIELD_ARGUMENTS
}

schema {
	query: Query
	mutation: Mutation
}

type Country {
  code: String
  name: String
  native: String
  phone: String
  continent: Continent
  currency: String
  languages: [Language]
  emoji: String
  emojiU: String
}

type Continent {
  code: String
  name: String
  countries: [Country]
}

type Language {
  code: String
  name: String
  native: String
  rtl: Int
}

type Query {
	country(code: String!): Country
		@GraphQLDataSource(
			host: "countries.trevorblades.com"
			url: "/"
			params: [
				{
					name: "code"
					sourceKind: FIELD_ARGUMENTS
					sourceName: "code"
					variableType: "String!"
				}
			]
		)
}

type Mutation {
	likePost(id: ID!): Post
		@GraphQLDataSource(
			host: "fakebook.com"
			url: "/"
			params: [
				{
					name: "id"
					sourceKind: FIELD_ARGUMENTS
					sourceName: "id"
					variableType: "ID!"
				}
			]
		)
}

type Post {
	id: ID!
	likes: Int!
}
`

const HTTPJSONDataSourceSchema = `
directive @HttpJsonDataSource (
    host: String!
    url: String!
    method: HTTP_METHOD = GET
    params: [Parameter]
	body: String
) on FIELD_DEFINITION

directive @mapTo(
	objectField: String!
) on FIELD_DEFINITION

directive @path(append: [String!], prepend: [String!]) on FIELD_DEFINITION

enum HTTP_METHOD {
    GET
    POST
    UPDATE
    DELETE
}

input Parameter {
    name: String!
    sourceKind: PARAMETER_SOURCE!
    sourceName: String!
    variableType: String!
}

enum PARAMETER_SOURCE {
    CONTEXT_VARIABLE
    OBJECT_VARIABLE_ARGUMENT
    FIELD_ARGUMENTS
}

schema {
    query: Query
}

type Foo {
    bar: String!
}

type Headers {
    Accept: String!
    Host: String!
	acceptEncoding: String @mapTo(objectField: "Accept-Encoding")
}

type HttpBinGet {
	header: Headers! @mapTo(objectField: "headers")
}

type JSONPlaceholderPost {
    userId: Int!
    id: Int!
    title: String!
    body: String!
    comments: [JSONPlaceholderComment]
        @HttpJsonDataSource(
            host: "jsonplaceholder.typicode.com"
            url: "/comments?postId={{ .postId }}"
            params: [
                {
                    name: "postId"
                    sourceKind: OBJECT_VARIABLE_ARGUMENT
                    sourceName: "id"
                    variableType: "String"
                }
            ]
        )
}

type JSONPlaceholderComment {
    postId: Int!
    id: Int!
    name: String!
    email: String!
    body: String!
}

"The query type, represents all of the entry points into our object graph"
type Query {
    httpBinGet: HttpBinGet
        @HttpJsonDataSource(
            host: "httpbin.org"
            url: "/get"
        )
	post(id: Int!): JSONPlaceholderPost
        @HttpJsonDataSource(
            host: "jsonplaceholder.typicode.com"
            url: "/posts/{{ .arguments.id }}"
        )
	withBody(input: WithBodyInput!): String!
        @HttpJsonDataSource(
            host: "httpbin.org"
            url: "/anything"
            method: POST
            body: 	"{\"key\":\"{{ .arguments.input.foo }}\"}"
        )
	withHeaders: String!
        @HttpJsonDataSource(
            host: "httpbin.org"
            url: "/anything"
            headers: [
				{
					key: "Authorization",
					value: "123",
				},
				{
					key: "Accept-Encoding",
					value: "application/json",
				}
			]
        )
	withPath: String!
		@path(prepend: ["subObject"])
		@HttpJsonDataSource(
            host: "httpbin.org"
            url: "/anything"
        )
    __schema: __Schema!
    __type(name: String!): __Type
}

input WithBodyInput {
	foo: String!
}
`

const staticDataSourceSchema = `
schema {
    query: Query
}

type Foo {
	bar: String!
}

"The query type, represents all of the entry points into our object graph"
type Query {
    hello: String!
		@StaticDataSource(
            data: "World!"
        )
	nullableInt: Int
        @StaticDataSource(
            data: null
        )
	foo: Foo!
        @StaticDataSource(
            data: "{\"bar\":\"baz\"}"
        )
}`

const complexSchema = `
directive @HttpJsonDataSource (
    host: String!
    url: String!
    method: HTTP_METHOD = GET
    params: [Parameter]
) on FIELD_DEFINITION

directive @GraphQLDataSource (
    host: String!
    url: String!
	method: HTTP_METHOD = POST
    params: [Parameter]
) on FIELD_DEFINITION

directive @mapTo(
	objectField: String!
) on FIELD_DEFINITION

enum HTTP_METHOD {
    GET
    POST
    UPDATE
    DELETE
}

input Parameter {
    name: String!
    sourceKind: PARAMETER_SOURCE!
    sourceName: String!
    variableType: String!
}

enum PARAMETER_SOURCE {
    CONTEXT_VARIABLE
    OBJECT_VARIABLE_ARGUMENT
    FIELD_ARGUMENTS
}

directive @resolveType (
	params: [Parameter]
) on FIELD_DEFINITION

directive @resolveSchema on FIELD_DEFINITION

enum HTTP_METHOD {
	GET
	POST
	UPDATE
	DELETE
}

input Parameter {
    name: String!
    sourceKind: PARAMETER_SOURCE!
    sourceName: String!
    variableType: String!
}

enum PARAMETER_SOURCE {
    CONTEXT_VARIABLE
    OBJECT_VARIABLE_ARGUMENT
    FIELD_ARGUMENTS
}

scalar Date

schema {
	query: Query
}

type Query {
	__type(name: String!): __Type!
		@resolveType(
			params: [
				{
					name: "name"
					sourceKind: FIELD_ARGUMENTS
					sourceName: "name"
					variableType: "String!"
				}
			]
		)
	__schema: __Schema!
	user(id: String!): User
		@GraphQLDataSource(
			host: "localhost:8001"
			url: "/graphql"
			params: [
				{
					name: "id"
					sourceKind: FIELD_ARGUMENTS
					sourceName: "id"
					variableType: "String!"
				}
			]
		)
	restUser(id: String!): User
		@HttpJsonDataSource (
			host: "localhost:9001"
			url: "/user/{{ .arguments.id }}"
		)
}
type User {
	id: String
	name: String
	birthday: Date
	friends: [User]
		@HttpJsonDataSource(
			host: "localhost:9000"
			url: "/user/:id/friends"
			params: [
				{
					name: "id"
					sourceKind: OBJECT_VARIABLE_ARGUMENT
					sourceName: "id"
					variableType: "String!"
				}
			]
		)
	pets: [Pet]
		@mapTo(
			objectField: "userPets"
		)
		@GraphQLDataSource(
			host: "localhost:8002"
			url: "/graphql"
			params: [
				{
					name: "userId"
					sourceKind: OBJECT_VARIABLE_ARGUMENT
					sourceName: "id"
					variableType: "String!"
				}
			]
		)
}
interface Pet {
	nickname: String!
}
type Dog implements Pet {
	name: String!
	nickname: String!
	woof: String!
}
type Cat implements Pet {
	name: String!
	nickname: String!
	meow: String!
}
`

func withBaseSchema(input string) string {
	return input + `
"The 'Int' scalar type represents non-fractional signed whole numeric values. Int can represent values between -(2^31) and 2^31 - 1."
scalar Int
"The 'Float' scalar type represents signed double-precision fractional values as specified by [IEEE 754](http://en.wikipedia.org/wiki/IEEE_floating_point)."
scalar Float
"The 'String' scalar type represents textual data, represented as UTF-8 character sequences. The String type is most often used by GraphQL to represent free-form human-readable text."
scalar String
"The 'Boolean' scalar type represents 'true' or 'false' ."
scalar Boolean
"The 'ID' scalar type represents a unique identifier, often used to refetch an object or as key for a cache. The ID type appears in a JSON response as a String; however, it is not intended to be human-readable. When expected as an input type, any string (such as '4') or integer (such as 4) input value will be accepted as an ID."
scalar ID @custom(typeName: "string")
"Directs the executor to include this field or fragment only when the argument is true."
directive @include(
    " Included when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Directs the executor to skip this field or fragment when the argument is true."
directive @skip(
    "Skipped when true."
    if: Boolean!
) on FIELD | FRAGMENT_SPREAD | INLINE_FRAGMENT
"Marks an element of a GraphQL schema as no longer supported."
directive @deprecated(
    """
    Explains why this element was deprecated, usually also including a suggestion
    for how to access supported similar data. Formatted in
    [Markdown](https://daringfireball.net/projects/markdown/).
    """
    reason: String = "No longer supported"
) on FIELD_DEFINITION | ENUM_VALUE

"""
A Directive provides a way to describe alternate runtime execution and type validation behavior in a GraphQL document.
In some cases, you need to provide options to alter GraphQL's execution behavior
in ways field arguments will not suffice, such as conditionally including or
skipping a field. Directives provide this by describing additional information
to the executor.
"""
type __Directive {
    name: String!
    description: String
    locations: [__DirectiveLocation!]!
    args: [__InputValue!]!
}

"""
A Directive can be adjacent to many parts of the GraphQL language, a
__DirectiveLocation describes one such possible adjacencies.
"""
enum __DirectiveLocation {
    "Location adjacent to a query operation."
    QUERY
    "Location adjacent to a mutation operation."
    MUTATION
    "Location adjacent to a subscription operation."
    SUBSCRIPTION
    "Location adjacent to a field."
    FIELD
    "Location adjacent to a fragment definition."
    FRAGMENT_DEFINITION
    "Location adjacent to a fragment spread."
    FRAGMENT_SPREAD
    "Location adjacent to an inline fragment."
    INLINE_FRAGMENT
    "Location adjacent to a schema definition."
    SCHEMA
    "Location adjacent to a scalar definition."
    SCALAR
    "Location adjacent to an object type definition."
    OBJECT
    "Location adjacent to a field definition."
    FIELD_DEFINITION
    "Location adjacent to an argument definition."
    ARGUMENT_DEFINITION
    "Location adjacent to an interface definition."
    INTERFACE
    "Location adjacent to a union definition."
    UNION
    "Location adjacent to an enum definition."
    ENUM
    "Location adjacent to an enum value definition."
    ENUM_VALUE
    "Location adjacent to an input object type definition."
    INPUT_OBJECT
    "Location adjacent to an input object field definition."
    INPUT_FIELD_DEFINITION
}
"""
One possible value for a given Enum. Enum values are unique values, not a
placeholder for a string or numeric value. However an Enum value is returned in
a JSON response as a string.
"""
type __EnumValue {
    name: String!
    description: String
    isDeprecated: Boolean!
    deprecationReason: String
}

"""
ObjectKind and Interface types are described by a list of Fields, each of which has
a name, potentially a list of arguments, and a return type.
"""
type __Field {
    name: String!
    description: String
    args: [__InputValue!]!
    type: __Type!
    isDeprecated: Boolean!
    deprecationReason: String
}

"""Arguments provided to Fields or Directives and the input fields of an
InputObject are represented as Input Values which describe their type and
optionally a default value.
"""
type __InputValue {
    name: String!
    description: String
    type: __Type!
    "A GraphQL-formatted string representing the default value for this input value."
    defaultValue: String
}

"""
A GraphQL Schema defines the capabilities of a GraphQL server. It exposes all
available types and directives on the server, as well as the entry points for
query, mutation, and subscription operations.
"""
type __Schema {
    "A list of all types supported by this server."
    types: [__Type!]!
    "The type that query operations will be rooted at."
    queryType: __Type!
    "If this server supports mutation, the type that mutation operations will be rooted at."
    mutationType: __Type
    "If this server support subscription, the type that subscription operations will be rooted at."
    subscriptionType: __Type
    "A list of all directives supported by this server."
    directives: [__Directive!]!
}

"""
The fundamental unit of any GraphQL Schema is the type. There are many kinds of
types in GraphQL as represented by the '__TypeKind' enum.

Depending on the kind of a type, certain fields describe information about that
type. Scalar types provide no information beyond a name and description, while
Enum types provide their values. ObjectKind and Interface types provide the fields
they describe. Abstract types, Union and Interface, provide the ObjectKind types
possible at runtime. ListKind and NonNull types compose other types.
"""
type __Type {
    kind: __TypeKind!
    name: String
    description: String
    fields(includeDeprecated: Boolean = false): [__Field!]
    interfaces: [__Type!]
    possibleTypes: [__Type!]
    enumValues(includeDeprecated: Boolean = false): [__EnumValue!]
    inputFields: [__InputValue!]
    ofType: __Type
}

"An enum describing what kind of type a given '__Type' is."
enum __TypeKind {
    "Indicates this type is a scalar."
    SCALAR
    "Indicates this type is an object. 'fields' and 'interfaces' are valid fields."
    OBJECT
    "Indicates this type is an interface. 'fields' ' and ' 'possibleTypes' are valid fields."
    INTERFACE
    "Indicates this type is a union. 'possibleTypes' is a valid field."
    UNION
    "Indicates this type is an enum. 'enumValues' is a valid field."
    ENUM
    "Indicates this type is an input object. 'inputFields' is a valid field."
    INPUT_OBJECT
    "Indicates this type is a list. 'ofType' is a valid field."
    LIST
    "Indicates this type is a non-null. 'ofType' is a valid field."
    NON_NULL
}
`
}

var kindNameDeepFields = []Field{
	{
		Name: []byte("kind"),
		Value: &Value{
			Path:       []string{"kind"},
			QuoteValue: true,
		},
	},
	{
		Name: []byte("name"),
		Value: &Value{
			Path:       []string{"name"},
			QuoteValue: true,
		},
	},
	{
		Name: []byte("ofType"),
		Value: &Object{
			Path: []string{"ofType"},
			Fields: []Field{
				{
					Name: []byte("kind"),
					Value: &Value{
						Path:       []string{"kind"},
						QuoteValue: true,
					},
				},
				{
					Name: []byte("name"),
					Value: &Value{
						Path:       []string{"name"},
						QuoteValue: true,
					},
				},
				{
					Name: []byte("ofType"),
					Value: &Object{
						Path: []string{"ofType"},
						Fields: []Field{
							{
								Name: []byte("kind"),
								Value: &Value{
									Path:       []string{"kind"},
									QuoteValue: true,
								},
							},
							{
								Name: []byte("name"),
								Value: &Value{
									Path:       []string{"name"},
									QuoteValue: true,
								},
							},
							{
								Name: []byte("ofType"),
								Value: &Object{
									Path: []string{"ofType"},
									Fields: []Field{
										{
											Name: []byte("kind"),
											Value: &Value{
												Path:       []string{"kind"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												Path:       []string{"name"},
												QuoteValue: true,
											},
										},
										{
											Name: []byte("ofType"),
											Value: &Object{
												Path: []string{"ofType"},
												Fields: []Field{
													{
														Name: []byte("kind"),
														Value: &Value{
															Path:       []string{"kind"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															Path:       []string{"name"},
															QuoteValue: true,
														},
													},
													{
														Name: []byte("ofType"),
														Value: &Object{
															Path: []string{"ofType"},
															Fields: []Field{
																{
																	Name: []byte("kind"),
																	Value: &Value{
																		Path:       []string{"kind"},
																		QuoteValue: true,
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &Value{
																		Path:       []string{"name"},
																		QuoteValue: true,
																	},
																},
																{
																	Name: []byte("ofType"),
																	Value: &Object{
																		Path: []string{"ofType"},
																		Fields: []Field{
																			{
																				Name: []byte("kind"),
																				Value: &Value{
																					Path:       []string{"kind"},
																					QuoteValue: true,
																				},
																			},
																			{
																				Name: []byte("name"),
																				Value: &Value{
																					Path:       []string{"name"},
																					QuoteValue: true,
																				},
																			},
																			{
																				Name: []byte("ofType"),
																				Value: &Object{
																					Path: []string{"ofType"},
																					Fields: []Field{
																						{
																							Name: []byte("kind"),
																							Value: &Value{
																								Path:       []string{"kind"},
																								QuoteValue: true,
																							},
																						},
																						{
																							Name: []byte("name"),
																							Value: &Value{
																								Path:       []string{"name"},
																								QuoteValue: true,
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}}
