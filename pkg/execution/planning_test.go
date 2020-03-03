package execution

import (
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().Unix())
}

func toJSON(any interface{}) []byte {
	data, _ := json.Marshal(any)
	return data
}

func stringPtr(str string) *string {
	return &str
}

func intPtr(i int) *int {
	return &i
}

func panicOnErr(err error) {
	if err != nil {
		panic(err)
	}
}

func run(definition string, operation string, configureBase func(base *BaseDataSourcePlanner), want Node, skip ...bool) func(t *testing.T) {
	return func(t *testing.T) {

		if len(skip) == 1 && skip[0] {
			return
		}

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)

		var report operationreport.Report
		normalizer := astnormalization.NewNormalizer(true)
		normalizer.NormalizeOperation(&op, &def, &report)
		if report.HasErrors() {
			t.Error(report)
		}

		base, err := NewBaseDataSourcePlanner([]byte(definition), PlannerConfiguration{}, log.NoopLogger)
		if err != nil {
			t.Fatal(err)
		}

		configureBase(base)

		planner := NewPlanner(base)
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

func TestPlanner_Plan(t *testing.T) {

	t.Run("GraphQLDataSource", run(withBaseSchema(GraphQLDataSourceSchema), `
				query GraphQLQuery($code: String!) {
					country(code: $code) {
						code
						name
						aliased: native
					}
				}
`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "country",
						DataSource: DataSourceConfig{
							Name: "GraphQLDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(GraphQLDataSourceConfig{
									Host: "countries.trevorblades.com",
									URL:  "/",
								})
								return data
							}(),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", GraphQLDataSourcePlannerFactoryFactory{}))
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
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&ContextVariableArgument{
										Name:         []byte("code"),
										VariableName: []byte("code"),
									},
								},
								DataSource: &GraphQLDataSource{
									log: log.NoopLogger,
								},
							},
							BufferName: "country",
						},
						Fields: []Field{
							{
								Name:            []byte("country"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "country",
										},
									},
									Fields: []Field{
										{
											Name: []byte("code"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "code",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("aliased"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "native",
													},
												},
												ValueType: StringValueType,
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
`, func(base *BaseDataSourcePlanner) {
		base.config = PlannerConfiguration{
			TypeFieldConfigurations: []TypeFieldConfiguration{
				{
					TypeName:  "mutation",
					FieldName: "likePost",
					DataSource: DataSourceConfig{
						Name: "GraphQLDataSource",
						Config: func() []byte {
							data, _ := json.Marshal(GraphQLDataSourceConfig{
								Host: "fakebook.com",
								URL:  "/",
							})
							return data
						}(),
					},
				},
			},
		}
		panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", GraphQLDataSourcePlannerFactoryFactory{}))
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
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{
									log: log.NoopLogger,
								},
							},
							BufferName: "likePost",
						},
						Fields: []Field{
							{
								Name:            []byte("likePost"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "likePost",
										},
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("likes"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "likes",
													},
												},
												ValueType: IntegerValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "JSONPlaceholderPost",
						FieldName: "id",
						Mapping: &MappingConfiguration{
							Path: "postId",
						},
					},
					{
						TypeName:  "JSONPlaceholderPost",
						FieldName: "comments",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "jsonplaceholder.typicode.com",
								URL:  "/comments?postId={{ .object.id }}",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "httpBinGet",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/get",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "post",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "jsonplaceholder.typicode.com",
								URL:  "/posts/{{ .arguments.id }}",
							}),
						},
					},
					{
						TypeName:  "HttpBinGet",
						FieldName: "header",
						Mapping: &MappingConfiguration{
							Path: "headers",
						},
					},
					{
						TypeName:  "Headers",
						FieldName: "acceptEncoding",
						Mapping: &MappingConfiguration{
							Path: "Accept-Encoding",
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
										DataSource: &HttpJsonDataSource{
											log: log.NoopLogger,
										},
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
											&StaticVariableArgument{
												Name:  []byte("__typename"),
												Value: []byte(`{"defaultTypeName":"HttpBinGet"}`),
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
											&StaticVariableArgument{
												Name:  []byte("__typename"),
												Value: []byte(`{"defaultTypeName":"JSONPlaceholderPost"}`),
											},
											&ContextVariableArgument{
												Name:         []byte(".arguments.id"),
												VariableName: []byte("id"),
											},
										},
										DataSource: &HttpJsonDataSource{
											log: log.NoopLogger,
										},
									},
									BufferName: "post",
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("httpBinGet"),
								HasResolvedData: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("header"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "headers",
													},
												},
												Fields: []Field{
													{
														Name: []byte("Accept"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "Accept",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("Host"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "Host",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("acceptEncoding"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "Accept-Encoding",
																},
															},
															ValueType: StringValueType,
														},
													},
												},
											},
										},
									},
								},
							},
							{
								Name:            []byte("post"),
								HasResolvedData: true,
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
													Value: []byte("/comments?postId={{ .object.id }}"),
												},
												&StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
												&StaticVariableArgument{
													Name:  []byte("__typename"),
													Value: []byte(`{"defaultTypeName":"JSONPlaceholderComment"}`),
												},
											},
											DataSource: &HttpJsonDataSource{
												log: log.NoopLogger,
											},
										},
										BufferName: "comments",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "postId",
													},
												},
												ValueType: IntegerValueType,
											},
										},
										{
											Name:            []byte("comments"),
											HasResolvedData: true,
											Value: &List{
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("id"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "id",
																	},
																},
																ValueType: IntegerValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withBody",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host:   "httpbin.org",
								URL:    "/anything",
								Method: stringPtr("POST"),
								Body:   stringPtr(`{\"key\":\"{{ .arguments.input.foo }}\"}`),
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
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
								Name:            []byte("withBody"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withPath",
						Mapping: &MappingConfiguration{
							Path: "subObject",
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
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
								Name:            []byte("withPath"),
								HasResolvedData: true,
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "subObject",
										},
									},
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		}))
	t.Run("HTTPJSONDataSource list withoutPath", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query ListWithoutPath {
						listItems {
							id
						}
					}
					`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listItems",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "listItems",
							Source: &DataSourceInvocation{
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
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
									&StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"ListItem"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("listItems"),
								HasResolvedData: true,
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "id",
														},
													},
													ValueType: StringValueType,
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
	t.Run("HTTPJSONDataSource list withPath", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query ListWithPath {
						listWithPath {
							id
						}
					}
					`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listWithPath",
						Mapping: &MappingConfiguration{
							Path: "items",
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "listWithPath",
							Source: &DataSourceInvocation{
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
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
									&StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"ListItem"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("listWithPath"),
								HasResolvedData: true,
								Value: &List{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "items",
										},
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "id",
														},
													},
													ValueType: StringValueType,
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
	t.Run("HTTPJSONDataSource withHeaders", run(withBaseSchema(HTTPJSONDataSourceSchema), `
					query WithHeader {
						withHeaders
					}
					`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withHeaders",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
								Headers: []HttpJsonDataSourceConfigHeader{
									{
										Key:   "Authorization",
										Value: "123",
									},
									{
										Key:   "Accept-Encoding",
										Value: "application/json",
									},
								},
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
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
								Name:            []byte("withHeaders"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "hello",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(StaticDataSourceConfig{
								Data: "World!",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "foo",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(StaticDataSourceConfig{
								Data: "{\"bar\":\"baz\"}",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "nullableInt",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(StaticDataSourceConfig{
								Data: "null",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", StaticDataSourcePlannerFactoryFactory{}))
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
										DataSource: &StaticDataSource{
											data: []byte("World!"),
										},
									},
									BufferName: "hello",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										DataSource: &StaticDataSource{
											data: []byte("null"),
										},
									},
									BufferName: "nullableInt",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										DataSource: &StaticDataSource{
											data: []byte("{\"bar\":\"baz\"}"),
										},
									},
									BufferName: "foo",
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("hello"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
								},
							},
							{
								Name:            []byte("nullableInt"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: IntegerValueType,
								},
							},
							{
								Name:            []byte("foo"),
								HasResolvedData: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("bar"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "bar",
													},
												},
												ValueType: StringValueType,
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
`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "__type",
						DataSource: DataSourceConfig{
							Name:   "TypeDataSource",
							Config: toJSON(TypeDataSourcePlannerConfig{}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("TypeDataSource", TypeDataSourcePlannerFactoryFactory{}))
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
									&ContextVariableArgument{
										Name:         []byte(".arguments.name"),
										VariableName: []byte("name"),
									},
								},
								DataSource: &TypeDataSource{},
							},
							BufferName: "__type",
						},
						Fields: []Field{
							{
								Name:            []byte("__type"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "__type",
										},
									},
									Fields: []Field{
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("fields"),
											Value: &List{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "fields",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("type"),
															Value: &Object{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "type",
																	},
																},
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "user",
						DataSource: DataSourceConfig{
							Name: "GraphQLDataSource",
							Config: toJSON(GraphQLDataSourceConfig{
								Host: "localhost:8001",
								URL:  "/graphql",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", GraphQLDataSourcePlannerFactoryFactory{}))
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
									&StaticVariableArgument{
										Name:  literal.METHOD,
										Value: []byte("POST"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{
									log: log.NoopLogger,
								},
							},
							BufferName: "user",
						},
						Fields: []Field{
							{
								Name:            []byte("user"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "user",
										},
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "birthday",
													},
												},
												ValueType: StringValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "restUser",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "localhost:9001",
								URL:  "/user/{{ .arguments.id }}",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
									&StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"User"}`),
									},
									&ContextVariableArgument{
										Name:         []byte(".arguments.id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &HttpJsonDataSource{
									log: log.NoopLogger,
								},
							},
							BufferName: "restUser",
						},
						Fields: []Field{
							{
								Name:            []byte("restUser"),
								HasResolvedData: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "birthday",
													},
												},
												ValueType: StringValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "user",
						DataSource: DataSourceConfig{
							Name: "GraphQLDataSource",
							Config: toJSON(GraphQLDataSourceConfig{
								Host: "localhost:8001",
								URL:  "/graphql",
							}),
						},
					},
					{
						TypeName:  "User",
						FieldName: "friends",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(HttpJsonDataSourceConfig{
								Host: "localhost:9001",
								URL:  "/user/{{ .object.id }}/friends",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", GraphQLDataSourcePlannerFactoryFactory{}))
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
									&StaticVariableArgument{
										Name:  literal.METHOD,
										Value: []byte("POST"),
									},
									&ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &GraphQLDataSource{
									log: log.NoopLogger,
								},
							},
							BufferName: "user",
						},
						Fields: []Field{
							{
								Name:            []byte("user"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "user",
										},
									},
									Fetch: &SingleFetch{
										Source: &DataSourceInvocation{
											Args: []Argument{
												&StaticVariableArgument{
													Name:  literal.HOST,
													Value: []byte("localhost:9001"),
												},
												&StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/user/{{ .object.id }}/friends"),
												},
												&StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
												&StaticVariableArgument{
													Name:  []byte("__typename"),
													Value: []byte(`{"defaultTypeName":"User"}`),
												},
											},
											DataSource: &HttpJsonDataSource{
												log: log.NoopLogger,
											},
										},
										BufferName: "friends",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("birthday"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "birthday",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name:            []byte("friends"),
											HasResolvedData: true,
											Value: &List{
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("id"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "id",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("birthday"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "birthday",
																	},
																},
																ValueType: StringValueType,
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
	/*	t.Run("nested rest and graphql resolver", run(withBaseSchema(complexSchema), ` //TODO: enable & implement #193
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
					return NewGraphQLDataSourcePlanner(BaseDataSourcePlanner{
						dataSourceConfig: PlannerConfiguration{
							TypeFieldConfigurations: []TypeFieldConfiguration{
								{
									TypeName:  "User",
									FieldName: "pets",
									Mapping: &MappingConfiguration{
										Path: "userPets",
									},
								},
							},
						},
					})
				},
			},
		},
		PlannerConfiguration{
			TypeFieldConfigurations: []TypeFieldConfiguration{
				{
					TypeName:  "User",
					FieldName: "pets",
					Mapping: &MappingConfiguration{
						Path: "userPets",
					},
					DataSource: DataSourceConfig{
						Name: "GraphQLDataSource",
						Config: toJSON(GraphQLDataSourceConfig{
							Host: "localhost:8002",
							URL:  "/graphql",
						}),
					},
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
				},
				{
					TypeName:  "query",
					FieldName: "user",
					DataSource: DataSourceConfig{
						Name: "GraphQLDataSource",
						Config: toJSON(GraphQLDataSourceConfig{
							Host: "localhost:8001",
							URL:  "/graphql",
						}),
					},
				},
				{
					TypeName:  "User",
					FieldName: "friends",
					Mapping: &MappingConfiguration{
						Disabled: true,
					},
					DataSource: DataSourceConfig{
						Name: "HttpJsonDataSource",
						Config: toJSON(HttpJsonDataSourceConfig{
							Host: "localhost:9001",
							URL:  "/user/{{ .object.id }}/friends",
						}),
					},
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
								Name:            []byte("user"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "user",
										},
									},
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
															PathSelector: PathSelector{
																Path: "id",
															},
														},
														&StaticVariableArgument{
															Name:  []byte("method"),
															Value: []byte("GET"),
														},
														&StaticVariableArgument{
															Name:  []byte("__typename"),
															Value: []byte(`{"defaultTypeName":"User"}`),
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
															PathSelector: PathSelector{
																Path: "id",
															},
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
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "id",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name:            []byte("friends"),
											HasResolvedData: true,
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
																	PathSelector: PathSelector{
																		Path: "id",
																	},
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
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "id",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("birthday"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "birthday",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name:            []byte("pets"),
															HasResolvedData: true,
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "userPets",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("__typename"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "__typename",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("nickname"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "nickname",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					PathSelector: PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("woof"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "woof",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					PathSelector: PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					PathSelector: PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &StaticVariableArgument{
																					Value: []byte("Cat"),
																				},
																			},
																		},
																		{
																			Name: []byte("meow"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "meow",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &ObjectVariableArgument{
																					PathSelector: PathSelector{
																						Path: "__typename",
																					},
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
											Name:            []byte("pets"),
											HasResolvedData: true,
											Value: &List{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "userPets",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("__typename"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "__typename",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("nickname"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "nickname",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	PathSelector: PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("woof"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "woof",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	PathSelector: PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	PathSelector: PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &StaticVariableArgument{
																	Value: []byte("Cat"),
																},
															},
														},
														{
															Name: []byte("meow"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "meow",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &ObjectVariableArgument{
																	PathSelector: PathSelector{
																		Path: "__typename",
																	},
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
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "birthday",
													},
												},
												ValueType: StringValueType,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}))*/
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "__schema",
						DataSource: DataSourceConfig{
							Name:   "SchemaDataSource",
							Config: toJSON(SchemaDataSourcePlannerConfig{}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("SchemaDataSource", SchemaDataSourcePlannerFactoryFactory{}))
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
								Name:            []byte("__schema"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "__schema",
										},
									},
									Fields: []Field{
										{
											Name: []byte("queryType"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "queryType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
												},
											},
										},
										{
											Name: []byte("mutationType"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "mutationType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
												},
											},
										},
										{
											Name: []byte("subscriptionType"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "subscriptionType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
												},
											},
										},
										{
											Name: []byte("types"),
											Value: &List{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "types",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("kind"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "kind",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("description"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "description",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("fields"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "fields",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "description",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("args"),
																			Value: &List{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "args",
																					},
																				},
																				Value: &Object{
																					Fields: []Field{
																						{
																							Name: []byte("name"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "name",
																									},
																								},
																								ValueType: StringValueType,
																							},
																						},
																						{
																							Name: []byte("description"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "description",
																									},
																								},
																								ValueType: StringValueType,
																							},
																						},
																						{
																							Name: []byte("type"),
																							Value: &Object{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "type",
																									},
																								},
																								Fields: kindNameDeepFields,
																							},
																						},
																						{
																							Name: []byte("defaultValue"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "defaultValue",
																									},
																								},
																								ValueType: StringValueType,
																							},
																						},
																					},
																				},
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "type",
																					},
																				},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("isDeprecated"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "isDeprecated",
																					},
																				},
																				ValueType: BooleanValueType,
																			},
																		},
																		{
																			Name: []byte("deprecationReason"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "deprecationReason",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("inputFields"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "inputFields",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "description",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "type",
																					},
																				},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("defaultValue"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "defaultValue",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("interfaces"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "interfaces",
																	},
																},
																Value: &Object{
																	Fields: kindNameDeepFields,
																},
															},
														},
														{
															Name: []byte("enumValues"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "enumValues",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "description",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("isDeprecated"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "isDeprecated",
																					},
																				},
																				ValueType: BooleanValueType,
																			},
																		},
																		{
																			Name: []byte("deprecationReason"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "deprecationReason",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("possibleTypes"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "possibleTypes",
																	},
																},
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
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "directives",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("description"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "description",
																	},
																},
																ValueType: StringValueType,
															},
														},
														{
															Name: []byte("locations"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "locations",
																	},
																},
																Value: &Value{
																	ValueType: StringValueType,
																},
															},
														},
														{
															Name: []byte("args"),
															Value: &List{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: PathSelector{
																		Path: "args",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("description"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "description",
																					},
																				},
																				ValueType: StringValueType,
																			},
																		},
																		{
																			Name: []byte("type"),
																			Value: &Object{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "type",
																					},
																				},
																				Fields: kindNameDeepFields,
																			},
																		},
																		{
																			Name: []byte("defaultValue"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: PathSelector{
																						Path: "defaultValue",
																					},
																				},
																				ValueType: StringValueType,
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
		true, // TODO: move into own test and unskip (currently skipped because schemaBytes marshal doesn't return comparable result (order of objects in JSON))
	))

	t.Run("http polling stream", run(withBaseSchema(HttpPollingStreamSchema), `
					subscription {
						stream {
							bar
							baz
						}
					}
`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "subscription",
						FieldName: "stream",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpPollingStreamDataSource",
							Config: toJSON(HttpPollingStreamDataSourceConfiguration{
								Host:         "foo.bar.baz",
								URL:          "/bal",
								DelaySeconds: intPtr(5),
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpPollingStreamDataSource", HttpPollingStreamDataSourcePlannerFactoryFactory{}))
		},
		&Object{
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
									log:   log.NoopLogger,
									delay: time.Second * 5,
								},
							},
							BufferName: "stream",
						},
						Fields: []Field{
							{
								Name:            []byte("stream"),
								HasResolvedData: true,
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("bar"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "bar",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("baz"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "baz",
													},
												},
												ValueType: IntegerValueType,
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
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "foos",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(StaticDataSourceConfig{
								Data: "[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", StaticDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &StaticDataSource{
									data: []byte("[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]"),
								},
							},
							BufferName: "foos",
						},
						Fields: []Field{
							{
								Name:            []byte("foos"),
								HasResolvedData: true,
								Value: &List{
									Filter: &ListFilterFirstN{
										FirstN: 2,
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("bar"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "bar",
														},
													},
													ValueType: StringValueType,
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
	t.Run("stringPipeline", run(withBaseSchema(pipelineSchema), `
			query PipelineQuery($foo: String!) {
				stringPipeline(foo: $foo)
			}
		`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "stringPipeline",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "PipelineDataSource",
							Config: toJSON(PipelineDataSourceConfig{
								ConfigString: stringPtr(`{
											"steps": [
												{
													"kind": "NOOP"
												}
											]
										}`),
								InputJSON: `{\"foo\":\"{{ .arguments.foo }}\"}`,
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("PipelineDataSource", PipelineDataSourcePlannerFactoryFactory{}))
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
										Name:  literal.INPUT_JSON,
										Value: []byte("{\\\"foo\\\":\\\"{{ .arguments.foo }}\\\"}"),
									},
									&ContextVariableArgument{
										Name:         []byte(".arguments.foo"),
										VariableName: []byte("foo"),
									},
								},
								DataSource: &PipelineDataSource{
									log: log.NoopLogger,
									pipeline: func() pipe.Pipeline {
										config := `{
														"steps": [
															{
																"kind": "NOOP",
																"dataSourceConfig": {
																	"template": "{\"result\":\"{{ .foo }}\"}"
																}
															}
														]
													}`
										var pipeline pipe.Pipeline
										err := pipeline.FromConfig(strings.NewReader(config))
										if err != nil {
											t.Fatal(err)
										}
										return pipeline
									}(),
								},
							},
							BufferName: "stringPipeline",
						},
						Fields: []Field{
							{
								Name:            []byte("stringPipeline"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("filePipeline", run(withBaseSchema(pipelineSchema), `
			query PipelineQuery($foo: String!) {
				filePipeline(foo: $foo)
			}
		`,
		func(base *BaseDataSourcePlanner) {
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "filePipeline",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "PipelineDataSource",
							Config: toJSON(PipelineDataSourceConfig{
								ConfigFilePath: stringPtr("./testdata/simple_pipeline.json"),
								InputJSON:      `{\"foo\":\"{{ .arguments.foo }}\"}`,
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("PipelineDataSource", PipelineDataSourcePlannerFactoryFactory{}))
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
										Name:  literal.INPUT_JSON,
										Value: []byte("{\\\"foo\\\":\\\"{{ .arguments.foo }}\\\"}"),
									},
									&ContextVariableArgument{
										Name:         []byte(".arguments.foo"),
										VariableName: []byte("foo"),
									},
								},
								DataSource: &PipelineDataSource{
									log: log.NoopLogger,
									pipeline: func() pipe.Pipeline {
										config := `{
														"steps": [
															{
																"kind": "NOOP"
															}
														]
													}`
										var pipeline pipe.Pipeline
										err := pipeline.FromConfig(strings.NewReader(config))
										if err != nil {
											t.Fatal(err)
										}
										return pipeline
									}(),
								},
							},
							BufferName: "filePipeline",
						},
						Fields: []Field{
							{
								Name:            []byte("filePipeline"),
								HasResolvedData: true,
								Value: &Value{
									ValueType: StringValueType,
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("unions", func(t *testing.T) {
		t.Run("getApis", run(UnionsSchema, `
			query getApis {
				apis {   
					... on ApisResultSuccess {
				  		apis {
							name
				  		}
					}
					... on RequestResult {
						status
						message
					}
			  	}
			}`,
			func(base *BaseDataSourcePlanner) {
				base.config = PlannerConfiguration{
					TypeFieldConfigurations: []TypeFieldConfiguration{
						{
							TypeName:  "query",
							FieldName: "apis",
							Mapping: &MappingConfiguration{
								Disabled: true,
							},
						},
					},
				}
			},
			&Object{
				operationType: ast.OperationTypeQuery,
				Fields: []Field{
					{
						Name: []byte("data"),
						Value: &Object{
							Fields: []Field{
								{
									Name: []byte("apis"),
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("apis"),
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														PathSelector: PathSelector{
															Path: "__typename",
														},
													},
													Right: &StaticVariableArgument{
														Value: []byte("ApisResultSuccess"),
													},
												},
												Value: &List{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "apis",
														},
													},
													Value: &Object{
														Fields: []Field{
															{
																Name: []byte("name"),
																Value: &Value{
																	DataResolvingConfig: DataResolvingConfig{
																		PathSelector: PathSelector{
																			Path: "name",
																		},
																	},
																	ValueType: StringValueType,
																},
															},
														},
													},
												},
											},
											{
												Name: []byte("status"),
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														PathSelector: PathSelector{
															Path: "__typename",
														},
													},
													Right: &StaticVariableArgument{
														Value: []byte("RequestResult"),
													},
												},
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "status",
														},
													},
												},
											},
											{
												Name: []byte("message"),
												Skip: &IfNotEqual{
													Left: &ObjectVariableArgument{
														PathSelector: PathSelector{
															Path: "__typename",
														},
													},
													Right: &StaticVariableArgument{
														Value: []byte("RequestResult"),
													},
												},
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: PathSelector{
															Path: "message",
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
	})
}

func BenchmarkPlanner_Plan(b *testing.B) {
	schema := withBaseSchema(complexSchema)
	def := unsafeparser.ParseGraphqlDocumentString(schema)
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

	config := PlannerConfiguration{
		TypeFieldConfigurations: []TypeFieldConfiguration{
			{
				TypeName:  "query",
				FieldName: "user",
				DataSource: DataSourceConfig{
					Name: "GraphQLDataSource",
					Config: toJSON(GraphQLDataSourceConfig{
						Host: "localhost:8001",
						URL:  "/graphql",
					}),
				},
			},
			{
				TypeName:  "User",
				FieldName: "friends",
				Mapping: &MappingConfiguration{
					Disabled: true,
				},
				DataSource: DataSourceConfig{
					Name: "HttpJsonDataSource",
					Config: toJSON(HttpJsonDataSourceConfig{
						Host: "localhost:9001",
						URL:  "/user/{{ .object.id }}/friends",
					}),
				},
			},
			{
				TypeName:  "query",
				FieldName: "user",
				DataSource: DataSourceConfig{
					Name: "GraphQLDataSource",
					Config: toJSON(GraphQLDataSourceConfig{
						Host: "localhost:8001",
						URL:  "/graphql",
					}),
				},
				Mapping: &MappingConfiguration{
					Path: "userPets",
				},
			},
		},
	}

	base, err := NewBaseDataSourcePlanner([]byte(schema), config, log.NoopLogger)
	if err != nil {
		b.Fatal(err)
	}

	planner := NewPlanner(base)
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

const UnionsSchema = `
schema {
	query: Query
}
type Query {
	api: ApiResult
	apis: ApisResult
}
type Api {
  id: String
  name: String
}
type RequestResult {
  message: String
  status: String
}
type ApisResultSuccess {
    apis: [Api!]
}
union ApisResult = ApisResultSuccess | RequestResult
union ApiResult = Api | RequestResult
scalar String
`

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
schema {
	subscription: Subscription
}

type Subscription {
	stream: Foo
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

enum MAPPING_MODE {
    NONE
    PATH_SELECTOR
}

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
}

type Mutation {
	likePost(id: ID!): Post
}

type Post {
	id: ID!
	likes: Int!
}
`

const HTTPJSONDataSourceSchema = `
enum HTTP_METHOD {
    GET
    POST
    UPDATE
    DELETE
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
	acceptEncoding: String
}

type HttpBinGet {
	header: Headers!
}

type JSONPlaceholderPost {
    userId: Int!
    id: Int!
    title: String!
    body: String!
    comments: [JSONPlaceholderComment]
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
	post(id: Int!): JSONPlaceholderPost
	withBody(input: WithBodyInput!): String!
	withHeaders: String!
	withPath: String!
	listItems: [ListItem]
	listWithPath: [ListItem]
    __schema: __Schema!
    __type(name: String!): __Type
}

type ListItem {
	id: String!
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
	nullableInt: Int
	foo: Foo!
}`

const complexSchema = `
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
	restUser(id: String!): User
}
type User {
	id: String
	name: String
	birthday: Date
	friends: [User]
	pets: [Pet]
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

const pipelineSchema = `
directive @PipelineDataSource (
    configFilePath: String
    configString: String
    inputJSON: String!
) on FIELD_DEFINITION

schema {
	query: Query
}

type Query {
	stringPipeline(foo: String!): String
	filePipeline(foo: String!): String
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

func introspectionQuery(schema []byte) RootNode {
	return &Object{
		operationType: ast.OperationTypeQuery,
		Fields: []Field{
			{
				Name: []byte("data"),
				Value: &Object{
					Fetch: &SingleFetch{
						Source: &DataSourceInvocation{
							DataSource: &SchemaDataSource{
								schemaBytes: schema,
							},
						},
						BufferName: "__schema",
					},
					Fields: []Field{
						{
							Name:            []byte("__schema"),
							HasResolvedData: true,
							Value: &Object{
								DataResolvingConfig: DataResolvingConfig{
									PathSelector: PathSelector{
										Path: "__schema",
									},
								},
								Fields: []Field{
									{
										Name: []byte("queryType"),
										Value: &Object{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: PathSelector{
													Path: "queryType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: PathSelector{
																Path: "name",
															},
														},
														ValueType: StringValueType,
													},
												},
											},
										},
									},
									{
										Name: []byte("mutationType"),
										Value: &Object{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: PathSelector{
													Path: "mutationType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: PathSelector{
																Path: "name",
															},
														},
														ValueType: StringValueType,
													},
												},
											},
										},
									},
									{
										Name: []byte("subscriptionType"),
										Value: &Object{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: PathSelector{
													Path: "subscriptionType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: PathSelector{
																Path: "name",
															},
														},
														ValueType: StringValueType,
													},
												},
											},
										},
									},
									{
										Name: []byte("types"),
										Value: &List{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: PathSelector{
													Path: "types",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("kind"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "kind",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("description"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "description",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("fields"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "fields",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("description"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "description",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("args"),
																		Value: &List{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "args",
																				},
																			},
																			Value: &Object{
																				Fields: []Field{
																					{
																						Name: []byte("name"),
																						Value: &Value{
																							DataResolvingConfig: DataResolvingConfig{
																								PathSelector: PathSelector{
																									Path: "name",
																								},
																							},
																							ValueType: StringValueType,
																						},
																					},
																					{
																						Name: []byte("description"),
																						Value: &Value{
																							DataResolvingConfig: DataResolvingConfig{
																								PathSelector: PathSelector{
																									Path: "description",
																								},
																							},
																							ValueType: StringValueType,
																						},
																					},
																					{
																						Name: []byte("type"),
																						Value: &Object{
																							DataResolvingConfig: DataResolvingConfig{
																								PathSelector: PathSelector{
																									Path: "type",
																								},
																							},
																							Fields: kindNameDeepFields,
																						},
																					},
																					{
																						Name: []byte("defaultValue"),
																						Value: &Value{
																							DataResolvingConfig: DataResolvingConfig{
																								PathSelector: PathSelector{
																									Path: "defaultValue",
																								},
																							},
																							ValueType: StringValueType,
																						},
																					},
																				},
																			},
																		},
																	},
																	{
																		Name: []byte("type"),
																		Value: &Object{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "type",
																				},
																			},
																			Fields: kindNameDeepFields,
																		},
																	},
																	{
																		Name: []byte("isDeprecated"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "isDeprecated",
																				},
																			},
																			ValueType: BooleanValueType,
																		},
																	},
																	{
																		Name: []byte("deprecationReason"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "deprecationReason",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																},
															},
														},
													},
													{
														Name: []byte("inputFields"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "inputFields",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("description"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "description",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("type"),
																		Value: &Object{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "type",
																				},
																			},
																			Fields: kindNameDeepFields,
																		},
																	},
																	{
																		Name: []byte("defaultValue"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "defaultValue",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																},
															},
														},
													},
													{
														Name: []byte("interfaces"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "interfaces",
																},
															},
															Value: &Object{
																Fields: kindNameDeepFields,
															},
														},
													},
													{
														Name: []byte("enumValues"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "enumValues",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("description"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "description",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("isDeprecated"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "isDeprecated",
																				},
																			},
																			ValueType: BooleanValueType,
																		},
																	},
																	{
																		Name: []byte("deprecationReason"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "deprecationReason",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																},
															},
														},
													},
													{
														Name: []byte("possibleTypes"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "possibleTypes",
																},
															},
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
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: PathSelector{
													Path: "directives",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("description"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "description",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("locations"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "locations",
																},
															},
															Value: &Value{
																ValueType: StringValueType,
															},
														},
													},
													{
														Name: []byte("args"),
														Value: &List{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "args",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "name",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("description"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "description",
																				},
																			},
																			ValueType: StringValueType,
																		},
																	},
																	{
																		Name: []byte("type"),
																		Value: &Object{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "type",
																				},
																			},
																			Fields: kindNameDeepFields,
																		},
																	},
																	{
																		Name: []byte("defaultValue"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: PathSelector{
																					Path: "defaultValue",
																				},
																			},
																			ValueType: StringValueType,
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
	}
}

var kindNameDeepFields = []Field{
	{
		Name: []byte("kind"),
		Value: &Value{
			DataResolvingConfig: DataResolvingConfig{
				PathSelector: PathSelector{
					Path: "kind",
				},
			},
			ValueType: StringValueType,
		},
	},
	{
		Name: []byte("name"),
		Value: &Value{
			DataResolvingConfig: DataResolvingConfig{
				PathSelector: PathSelector{
					Path: "name",
				},
			},
			ValueType: StringValueType,
		},
	},
	{
		Name: []byte("ofType"),
		Value: &Object{
			DataResolvingConfig: DataResolvingConfig{
				PathSelector: PathSelector{
					Path: "ofType",
				},
			},
			Fields: []Field{
				{
					Name: []byte("kind"),
					Value: &Value{
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: PathSelector{
								Path: "kind",
							},
						},
						ValueType: StringValueType,
					},
				},
				{
					Name: []byte("name"),
					Value: &Value{
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: PathSelector{
								Path: "name",
							},
						},
						ValueType: StringValueType,
					},
				},
				{
					Name: []byte("ofType"),
					Value: &Object{
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: PathSelector{
								Path: "ofType",
							},
						},
						Fields: []Field{
							{
								Name: []byte("kind"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "kind",
										},
									},
									ValueType: StringValueType,
								},
							},
							{
								Name: []byte("name"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "name",
										},
									},
									ValueType: StringValueType,
								},
							},
							{
								Name: []byte("ofType"),
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: PathSelector{
											Path: "ofType",
										},
									},
									Fields: []Field{
										{
											Name: []byte("kind"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "kind",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("ofType"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "ofType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("kind"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "kind",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "name",
																},
															},
															ValueType: StringValueType,
														},
													},
													{
														Name: []byte("ofType"),
														Value: &Object{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: PathSelector{
																	Path: "ofType",
																},
															},
															Fields: []Field{
																{
																	Name: []byte("kind"),
																	Value: &Value{
																		DataResolvingConfig: DataResolvingConfig{
																			PathSelector: PathSelector{
																				Path: "kind",
																			},
																		},
																		ValueType: StringValueType,
																	},
																},
																{
																	Name: []byte("name"),
																	Value: &Value{
																		DataResolvingConfig: DataResolvingConfig{
																			PathSelector: PathSelector{
																				Path: "name",
																			},
																		},
																		ValueType: StringValueType,
																	},
																},
																{
																	Name: []byte("ofType"),
																	Value: &Object{
																		DataResolvingConfig: DataResolvingConfig{
																			PathSelector: PathSelector{
																				Path: "ofType",
																			},
																		},
																		Fields: []Field{
																			{
																				Name: []byte("kind"),
																				Value: &Value{
																					DataResolvingConfig: DataResolvingConfig{
																						PathSelector: PathSelector{
																							Path: "kind",
																						},
																					},
																					ValueType: StringValueType,
																				},
																			},
																			{
																				Name: []byte("name"),
																				Value: &Value{
																					DataResolvingConfig: DataResolvingConfig{
																						PathSelector: PathSelector{
																							Path: "name",
																						},
																					},
																					ValueType: StringValueType,
																				},
																			},
																			{
																				Name: []byte("ofType"),
																				Value: &Object{
																					DataResolvingConfig: DataResolvingConfig{
																						PathSelector: PathSelector{
																							Path: "ofType",
																						},
																					},
																					Fields: []Field{
																						{
																							Name: []byte("kind"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "kind",
																									},
																								},
																								ValueType: StringValueType,
																							},
																						},
																						{
																							Name: []byte("name"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: PathSelector{
																										Path: "name",
																									},
																								},
																								ValueType: StringValueType,
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
