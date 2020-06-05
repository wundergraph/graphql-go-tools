package execution

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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

func runWithOperationName(definition string, operation string, operationName string, configureBase func(base *datasource.BasePlanner), want Node, skip ...bool) func(t *testing.T) {
	return func(t *testing.T) {

		if len(skip) == 1 && skip[0] {
			return
		}

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)

		var report operationreport.Report
		normalizer := astnormalization.NewNormalizer(true, true)
		normalizer.NormalizeOperation(&op, &def, &report)
		if report.HasErrors() {
			t.Error(report)
		}

		base, err := datasource.NewBaseDataSourcePlanner([]byte(definition), datasource.PlannerConfiguration{}, log.NoopLogger)
		if err != nil {
			t.Fatal(err)
		}

		configureBase(base)

		planner := NewPlanner(base)
		got := planner.Plan(&op, &def, operationName, &report)
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

func run(definition string, operation string, configureBase func(base *datasource.BasePlanner), want Node, skip ...bool) func(t *testing.T) {
	return runWithOperationName(definition, operation, "", configureBase, want, skip...)
}

func runAndReportExternalErrorWithOperationName(definition string, operation string, operationName string, configureBase func(base *datasource.BasePlanner), expectedError operationreport.ExternalError, skip ...bool) func(t *testing.T) {
	return func(t *testing.T) {

		if len(skip) == 1 && skip[0] {
			return
		}

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)

		var report operationreport.Report
		normalizer := astnormalization.NewNormalizer(true, true)
		normalizer.NormalizeOperation(&op, &def, &report)
		if report.HasErrors() {
			t.Error(report)
		}

		base, err := datasource.NewBaseDataSourcePlanner([]byte(definition), datasource.PlannerConfiguration{}, log.NoopLogger)
		if err != nil {
			t.Fatal(err)
		}

		configureBase(base)

		planner := NewPlanner(base)
		_ = planner.Plan(&op, &def, operationName, &report)
		assert.Error(t, report)
		require.Greater(t, len(report.ExternalErrors), 0)
		assert.Equal(t, expectedError.Message, report.ExternalErrors[0].Message)
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "country",
						DataSource: datasource.SourceConfig{
							Name: "GraphQLDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(datasource.GraphQLDataSourceConfig{
									Host: "countries.trevorblades.com",
									URL:  "/",
								})
								return data
							}(),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", datasource.GraphQLDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("countries.trevorblades.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("query"),
										Value: []byte("query o($code: String!){country(code: $code){code name native}}"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte("code"),
										VariableName: []byte("code"),
									},
								},
								DataSource: &datasource.GraphQLDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
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
										PathSelector: datasource.PathSelector{
											Path: "country",
										},
									},
									Fields: []Field{
										{
											Name: []byte("code"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
`, func(base *datasource.BasePlanner) {
		base.Config = datasource.PlannerConfiguration{
			TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
				{
					TypeName:  "mutation",
					FieldName: "likePost",
					DataSource: datasource.SourceConfig{
						Name: "GraphQLDataSource",
						Config: func() []byte {
							data, _ := json.Marshal(datasource.GraphQLDataSourceConfig{
								Host: "fakebook.com",
								URL:  "/",
							})
							return data
						}(),
					},
				},
			},
		}
		panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", datasource.GraphQLDataSourcePlannerFactoryFactory{}))
	},
		&Object{
			operationType: ast.OperationTypeMutation,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("fakebook.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("query"),
										Value: []byte("mutation o($id: ID!){likePost(id: $id){id likes}}"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &datasource.GraphQLDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
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
										PathSelector: datasource.PathSelector{
											Path: "likePost",
										},
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "JSONPlaceholderPost",
						FieldName: "id",
						Mapping: &datasource.MappingConfiguration{
							Path: "postId",
						},
					},
					{
						TypeName:  "JSONPlaceholderPost",
						FieldName: "comments",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "jsonplaceholder.typicode.com",
								URL:  "/comments?postId={{ .object.id }}",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "httpBinGet",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/get",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "post",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "jsonplaceholder.typicode.com",
								URL:  "/posts/{{ .arguments.id }}",
							}),
						},
					},
					{
						TypeName:  "HttpBinGet",
						FieldName: "header",
						Mapping: &datasource.MappingConfiguration{
							Path: "headers",
						},
					},
					{
						TypeName:  "Headers",
						FieldName: "acceptEncoding",
						Mapping: &datasource.MappingConfiguration{
							Path: "Accept-Encoding",
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
										DataSource: &datasource.HttpJsonDataSource{
											Log:    log.NoopLogger,
											Client: datasource.DefaultHttpClient(),
										},
										Args: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  []byte("host"),
												Value: []byte("httpbin.org"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("/get"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("method"),
												Value: []byte("GET"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("__typename"),
												Value: []byte(`{"defaultTypeName":"HttpBinGet"}`),
											},
										},
									},
									BufferName: "httpBinGet",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										Args: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  []byte("host"),
												Value: []byte("jsonplaceholder.typicode.com"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("/posts/{{ .arguments.id }}"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("method"),
												Value: []byte("GET"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("__typename"),
												Value: []byte(`{"defaultTypeName":"JSONPlaceholderPost"}`),
											},
											&datasource.ContextVariableArgument{
												Name:         []byte(".arguments.id"),
												VariableName: []byte("id"),
											},
										},
										DataSource: &datasource.HttpJsonDataSource{
											Log:    log.NoopLogger,
											Client: datasource.DefaultHttpClient(),
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
													PathSelector: datasource.PathSelector{
														Path: "headers",
													},
												},
												Fields: []Field{
													{
														Name: []byte("Accept"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
											Args: []datasource.Argument{
												&datasource.StaticVariableArgument{
													Name:  []byte("host"),
													Value: []byte("jsonplaceholder.typicode.com"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("url"),
													Value: []byte("/comments?postId={{ .object.id }}"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("__typename"),
													Value: []byte(`{"defaultTypeName":"JSONPlaceholderComment"}`),
												},
											},
											DataSource: &datasource.HttpJsonDataSource{
												Log:    log.NoopLogger,
												Client: datasource.DefaultHttpClient(),
											},
										},
										BufferName: "comments",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withBody",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host:   "httpbin.org",
								URL:    "/anything",
								Method: stringPtr("POST"),
								Body:   stringPtr(`{\"key\":\"{{ .arguments.input.foo }}\"}`),
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("POST"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("body"),
										Value: []byte("{\\\"key\\\":\\\"{{ .arguments.input.foo }}\\\"}"),
									},
									&datasource.ContextVariableArgument{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withPath",
						Mapping: &datasource.MappingConfiguration{
							Path: "subObject",
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&datasource.StaticVariableArgument{
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
										PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listItems",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
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
														PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listWithPath",
						Mapping: &datasource.MappingConfiguration{
							Path: "items",
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
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
										PathSelector: datasource.PathSelector{
											Path: "items",
										},
									},
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("id"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "withHeaders",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "httpbin.org",
								URL:  "/anything",
								Headers: []datasource.HttpJsonDataSourceConfigHeader{
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
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("httpbin.org"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/anything"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.ListArgument{
										Name: []byte("headers"),
										Arguments: []datasource.Argument{
											&datasource.StaticVariableArgument{
												Name:  []byte("Authorization"),
												Value: []byte("123"),
											},
											&datasource.StaticVariableArgument{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "hello",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(datasource.StaticDataSourceConfig{
								Data: "World!",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "foo",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(datasource.StaticDataSourceConfig{
								Data: "{\"bar\":\"baz\"}",
							}),
						},
					},
					{
						TypeName:  "query",
						FieldName: "nullableInt",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(datasource.StaticDataSourceConfig{
								Data: "null",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", datasource.StaticDataSourcePlannerFactoryFactory{}))
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
										DataSource: &datasource.StaticDataSource{
											Data: []byte("World!"),
										},
									},
									BufferName: "hello",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										DataSource: &datasource.StaticDataSource{
											Data: []byte("null"),
										},
									},
									BufferName: "nullableInt",
								},
								&SingleFetch{
									Source: &DataSourceInvocation{
										DataSource: &datasource.StaticDataSource{
											Data: []byte("{\"bar\":\"baz\"}"),
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "__type",
						DataSource: datasource.SourceConfig{
							Name:   "TypeDataSource",
							Config: toJSON(datasource.TypeDataSourcePlannerConfig{}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("TypeDataSource", datasource.TypeDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.ContextVariableArgument{
										Name:         []byte(".arguments.name"),
										VariableName: []byte("name"),
									},
								},
								DataSource: &datasource.TypeDataSource{},
							},
							BufferName: "__type",
						},
						Fields: []Field{
							{
								Name:            []byte("__type"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "__type",
										},
									},
									Fields: []Field{
										{
											Name: []byte("name"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "fields",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "type",
																	},
																},
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "user",
						DataSource: datasource.SourceConfig{
							Name: "GraphQLDataSource",
							Config: toJSON(datasource.GraphQLDataSourceConfig{
								Host: "localhost:8001",
								URL:  "/graphql",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", datasource.GraphQLDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.METHOD,
										Value: []byte("POST"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &datasource.GraphQLDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
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
										PathSelector: datasource.PathSelector{
											Path: "user",
										},
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "restUser",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "localhost:9001",
								URL:  "/user/{{ .arguments.id }}",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:9001"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/user/{{ .arguments.id }}"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"User"}`),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte(".arguments.id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &datasource.HttpJsonDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "user",
						DataSource: datasource.SourceConfig{
							Name: "GraphQLDataSource",
							Config: toJSON(datasource.GraphQLDataSourceConfig{
								Host: "localhost:8001",
								URL:  "/graphql",
							}),
						},
					},
					{
						TypeName:  "User",
						FieldName: "friends",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: toJSON(datasource.HttpJsonDataSourceConfig{
								Host: "localhost:9001",
								URL:  "/user/{{ .object.id }}/friends",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("GraphQLDataSource", datasource.GraphQLDataSourcePlannerFactoryFactory{}))
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", &datasource.HttpJsonDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.METHOD,
										Value: []byte("POST"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte("id"),
										VariableName: []byte("id"),
									},
								},
								DataSource: &datasource.GraphQLDataSource{
									Log:    log.NoopLogger,
									Client: datasource.DefaultHttpClient(),
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
										PathSelector: datasource.PathSelector{
											Path: "user",
										},
									},
									Fetch: &SingleFetch{
										Source: &DataSourceInvocation{
											Args: []datasource.Argument{
												&datasource.StaticVariableArgument{
													Name:  literal.HOST,
													Value: []byte("localhost:9001"),
												},
												&datasource.StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("/user/{{ .object.id }}/friends"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("method"),
													Value: []byte("GET"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("__typename"),
													Value: []byte(`{"defaultTypeName":"User"}`),
												},
											},
											DataSource: &datasource.HttpJsonDataSource{
												Log:    log.NoopLogger,
												Client: datasource.DefaultHttpClient(),
											},
										},
										BufferName: "friends",
									},
									Fields: []Field{
										{
											Name: []byte("id"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
				PlannerFactory: func() Planner {
					return NewGraphQLDataSourcePlanner(BasePlanner{})
				},
			},
			{
				TypeName:  []byte("User"),
				FieldName: []byte("friends"),
				PlannerFactory: func() Planner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
			{
				TypeName:  []byte("User"),
				FieldName: []byte("pets"),
				PlannerFactory: func() Planner {
					return NewGraphQLDataSourcePlanner(BasePlanner{
						dataSourceConfig: PlannerConfiguration{
							TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
								{
									TypeName:  "User",
									FieldName: "pets",
									Mapping: &datasource.MappingConfiguration{
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
			TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
				{
					TypeName:  "User",
					FieldName: "pets",
					Mapping: &datasource.MappingConfiguration{
						Path: "userPets",
					},
					DataSource: SourceConfig{
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
					DataSource: SourceConfig{
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
					Mapping: &datasource.MappingConfiguration{
						Disabled: true,
					},
					DataSource: SourceConfig{
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
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("localhost:8001"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/graphql"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.QUERY,
										Value: []byte("query o($id: String!){user(id: $id){id name birthday}}"),
									},
									&datasource.ContextVariableArgument{
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
										PathSelector: datasource.PathSelector{
											Path: "user",
										},
									},
									Fetch: &ParallelFetch{
										Fetches: []Fetch{
											&SingleFetch{
												Source: &DataSourceInvocation{
													Args: []datasource.Argument{
														&datasource.StaticVariableArgument{
															Name:  literal.HOST,
															Value: []byte("localhost:9000"),
														},
														&datasource.StaticVariableArgument{
															Name:  literal.URL,
															Value: []byte("/user/:id/friends"),
														},
														&datasource.ObjectVariableArgument{
															Name: []byte("id"),
															PathSelector: datasource.PathSelector{
																Path: "id",
															},
														},
														&datasource.StaticVariableArgument{
															Name:  []byte("method"),
															Value: []byte("GET"),
														},
														&datasource.StaticVariableArgument{
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
													Args: []datasource.Argument{
														&datasource.StaticVariableArgument{
															Name:  literal.HOST,
															Value: []byte("localhost:8002"),
														},
														&datasource.StaticVariableArgument{
															Name:  literal.URL,
															Value: []byte("/graphql"),
														},
														&datasource.StaticVariableArgument{
															Name:  literal.QUERY,
															Value: []byte("query o($userId: String!){userPets(userId: $userId){__typename nickname ... on Dog {name woof} ... on Cat {name meow}}}"),
														},
														&datasource.ObjectVariableArgument{
															Name: []byte("userId"),
															PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
															Args: []datasource.Argument{
																&datasource.StaticVariableArgument{
																	Name:  literal.HOST,
																	Value: []byte("localhost:8002"),
																},
																&datasource.StaticVariableArgument{
																	Name:  literal.URL,
																	Value: []byte("/graphql"),
																},
																&datasource.StaticVariableArgument{
																	Name:  literal.QUERY,
																	Value: []byte("query o($userId: String!){userPets(userId: $userId){__typename nickname ... on Dog {name woof} ... on Cat {name meow}}}"),
																},
																&datasource.ObjectVariableArgument{
																	Name: []byte("userId"),
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "userPets",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("__typename"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &datasource.ObjectVariableArgument{
																					PathSelector: datasource.PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &datasource.StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("woof"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
																						Path: "woof",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &datasource.ObjectVariableArgument{
																					PathSelector: datasource.PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &datasource.StaticVariableArgument{
																					Value: []byte("Dog"),
																				},
																			},
																		},
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
																						Path: "name",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &datasource.ObjectVariableArgument{
																					PathSelector: datasource.PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &datasource.StaticVariableArgument{
																					Value: []byte("Cat"),
																				},
																			},
																		},
																		{
																			Name: []byte("meow"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
																						Path: "meow",
																					},
																				},
																				ValueType: StringValueType,
																			},
																			Skip: &IfNotEqual{
																				Left: &datasource.ObjectVariableArgument{
																					PathSelector: datasource.PathSelector{
																						Path: "__typename",
																					},
																				},
																				Right: &datasource.StaticVariableArgument{
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
													PathSelector: datasource.PathSelector{
														Path: "userPets",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("__typename"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &datasource.ObjectVariableArgument{
																	PathSelector: datasource.PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &datasource.StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("woof"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
																		Path: "woof",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &datasource.ObjectVariableArgument{
																	PathSelector: datasource.PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &datasource.StaticVariableArgument{
																	Value: []byte("Dog"),
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
																		Path: "name",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &datasource.ObjectVariableArgument{
																	PathSelector: datasource.PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &datasource.StaticVariableArgument{
																	Value: []byte("Cat"),
																},
															},
														},
														{
															Name: []byte("meow"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
																		Path: "meow",
																	},
																},
																ValueType: StringValueType,
															},
															Skip: &IfNotEqual{
																Left: &datasource.ObjectVariableArgument{
																	PathSelector: datasource.PathSelector{
																		Path: "__typename",
																	},
																},
																Right: &datasource.StaticVariableArgument{
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "__schema",
						DataSource: datasource.SourceConfig{
							Name:   "SchemaDataSource",
							Config: toJSON(datasource.SchemaDataSourcePlannerConfig{}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("SchemaDataSource", datasource.SchemaDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &datasource.SchemaDataSource{},
							},
							BufferName: "__schema",
						},
						Fields: []Field{
							{
								Name:            []byte("__schema"),
								HasResolvedData: true,
								Value: &Object{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
											Path: "__schema",
										},
									},
									Fields: []Field{
										{
											Name: []byte("queryType"),
											Value: &Object{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "queryType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "mutationType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "subscriptionType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "types",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("kind"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "fields",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
																						Path: "args",
																					},
																				},
																				Value: &Object{
																					Fields: []Field{
																						{
																							Name: []byte("name"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: datasource.PathSelector{
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
																									PathSelector: datasource.PathSelector{
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
																									PathSelector: datasource.PathSelector{
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
																									PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "inputFields",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "enumValues",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "directives",
													},
												},
												Value: &Object{
													Fields: []Field{
														{
															Name: []byte("name"),
															Value: &Value{
																DataResolvingConfig: DataResolvingConfig{
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
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
																	PathSelector: datasource.PathSelector{
																		Path: "args",
																	},
																},
																Value: &Object{
																	Fields: []Field{
																		{
																			Name: []byte("name"),
																			Value: &Value{
																				DataResolvingConfig: DataResolvingConfig{
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
																					PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "subscription",
						FieldName: "stream",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpPollingStreamDataSource",
							Config: toJSON(datasource.HttpPollingStreamDataSourceConfiguration{
								Host:         "foo.bar.baz",
								URL:          "/bal",
								DelaySeconds: intPtr(5),
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpPollingStreamDataSource", datasource.HttpPollingStreamDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeSubscription,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.HOST,
										Value: []byte("foo.bar.baz"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("/bal"),
									},
								},
								DataSource: &datasource.HttpPollingStreamDataSource{
									Log:   log.NoopLogger,
									Delay: time.Second * 5,
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "foos",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "StaticDataSource",
							Config: toJSON(datasource.StaticDataSourceConfig{
								Data: "[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]",
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("StaticDataSource", datasource.StaticDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								DataSource: &datasource.StaticDataSource{
									Data: []byte("[{\"bar\":\"baz\"},{\"bar\":\"bal\"},{\"bar\":\"bat\"}]"),
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
														PathSelector: datasource.PathSelector{
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "stringPipeline",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "PipelineDataSource",
							Config: toJSON(datasource.PipelineDataSourceConfig{
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
			panicOnErr(base.RegisterDataSourcePlannerFactory("PipelineDataSource", datasource.PipelineDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.INPUT_JSON,
										Value: []byte("{\\\"foo\\\":\\\"{{ .arguments.foo }}\\\"}"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte(".arguments.foo"),
										VariableName: []byte("foo"),
									},
								},
								DataSource: &datasource.PipelineDataSource{
									Log: log.NoopLogger,
									Pipeline: func() pipe.Pipeline {
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
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "filePipeline",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "PipelineDataSource",
							Config: toJSON(datasource.PipelineDataSourceConfig{
								ConfigFilePath: stringPtr("./testdata/simple_pipeline.json"),
								InputJSON:      `{\"foo\":\"{{ .arguments.foo }}\"}`,
							}),
						},
					},
				},
			}
			panicOnErr(base.RegisterDataSourcePlannerFactory("PipelineDataSource", datasource.PipelineDataSourcePlannerFactoryFactory{}))
		},
		&Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fetch: &SingleFetch{
							Source: &DataSourceInvocation{
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  literal.INPUT_JSON,
										Value: []byte("{\\\"foo\\\":\\\"{{ .arguments.foo }}\\\"}"),
									},
									&datasource.ContextVariableArgument{
										Name:         []byte(".arguments.foo"),
										VariableName: []byte("foo"),
									},
								},
								DataSource: &datasource.PipelineDataSource{
									Log: log.NoopLogger,
									Pipeline: func() pipe.Pipeline {
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
			func(base *datasource.BasePlanner) {
				base.Config = datasource.PlannerConfiguration{
					TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
						{
							TypeName:  "query",
							FieldName: "apis",
							Mapping: &datasource.MappingConfiguration{
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
													Left: &datasource.ObjectVariableArgument{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
													Right: &datasource.StaticVariableArgument{
														Value: []byte("ApisResultSuccess"),
													},
												},
												Value: &List{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "apis",
														},
													},
													Value: &Object{
														Fields: []Field{
															{
																Name: []byte("name"),
																Value: &Value{
																	DataResolvingConfig: DataResolvingConfig{
																		PathSelector: datasource.PathSelector{
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
													Left: &datasource.ObjectVariableArgument{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
													Right: &datasource.StaticVariableArgument{
														Value: []byte("RequestResult"),
													},
												},
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "status",
														},
													},
												},
											},
											{
												Name: []byte("message"),
												Skip: &IfNotEqual{
													Left: &datasource.ObjectVariableArgument{
														PathSelector: datasource.PathSelector{
															Path: "__typename",
														},
													},
													Right: &datasource.StaticVariableArgument{
														Value: []byte("RequestResult"),
													},
												},
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
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

	t.Run("operation selection", func(t *testing.T) {
		var allCountriesPlannerConfig = func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "countries",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
					},
				},
			}
		}
		var addLanguagePlannerConfig = func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "Mutation",
						FieldName: "addLanguage",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
					},
				},
			}
		}
		var allCountriesQueryNode = &Object{
			operationType: ast.OperationTypeQuery,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []Field{
							{
								Name: []byte("countries"),
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("code"),
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "code",
														},
													},
												},
											},
											{
												Name: []byte("name"),
												Value: &Value{
													ValueType: StringValueType,
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "name",
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
		var addLanguageMutationNode = &Object{
			operationType: ast.OperationTypeMutation,
			Fields: []Field{
				{
					Name: []byte("data"),
					Value: &Object{
						Fields: []Field{
							{
								Name: []byte("addLanguage"),
								Value: &Object{
									Fields: []Field{
										{
											Name: []byte("code"),
											Value: &Value{
												ValueType: StringValueType,
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "code",
													},
												},
											},
										},
										{
											Name: []byte("name"),
											Value: &Value{
												ValueType: StringValueType,
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "name",
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

		t.Run("select single anonymous operation (case 1a in GraphQL spec)", runWithOperationName(countriesSchema, `
			{
				countries {
					code
					name
				}
			}`,
			"",
			allCountriesPlannerConfig,
			allCountriesQueryNode,
		))

		t.Run("select single named operation without name (case 1a in GraphQL spec)", runWithOperationName(countriesSchema, `
			query AllCountries {
				countries {
					code
					name
				}
			}`,
			"",
			allCountriesPlannerConfig,
			allCountriesQueryNode,
		))

		t.Run("select named operation / query with name (case 2a in GraphQL spec)", runWithOperationName(countriesSchema, `
			query AllContinents {
				continents {
					code
					name
				}
			}

			query AllCountries {
				countries {
					code
					name
				}
			}`,
			"AllCountries",
			allCountriesPlannerConfig,
			allCountriesQueryNode,
		))

		t.Run("select named operation / mutation with name (case 2a in GraphQL spec)", runWithOperationName(countriesSchema, `
			query AllContinents {
				continents {
					code
					name
				}
			}

			mutation AddLanguage {
				addLanguage(code: "GO", name: "go") {
					code
					name
				}
			}`,
			"AddLanguage",
			addLanguagePlannerConfig,
			addLanguageMutationNode,
		))

		t.Run("return error when multiple operations are available and operation name was not provided (case 1b in GraphQL spec)", runAndReportExternalErrorWithOperationName(countriesSchema, `
			query AllContinents {
				continents {
					code
					name
				}
			}

			query AllCountries {
				countries {
					code
					name
				}
			}`,
			"",
			allCountriesPlannerConfig,
			operationreport.ErrRequiredOperationNameIsMissing(),
		))

		t.Run("return error when multiple operations are available and operation name does not match (case 2b in GraphQL spec)", runAndReportExternalErrorWithOperationName(countriesSchema, `
			query AllContinents {
				continents {
					code
					name
				}
			}

			query AllCountries {
				countries {
					code
					name
				}
			}`,
			"NoQuery",
			allCountriesPlannerConfig,
			operationreport.ErrOperationWithProvidedOperationNameNotFound("NoQuery"),
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

	config := datasource.PlannerConfiguration{
		TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
			{
				TypeName:  "query",
				FieldName: "user",
				DataSource: datasource.SourceConfig{
					Name: "GraphQLDataSource",
					Config: toJSON(datasource.GraphQLDataSourceConfig{
						Host: "localhost:8001",
						URL:  "/graphql",
					}),
				},
			},
			{
				TypeName:  "User",
				FieldName: "friends",
				Mapping: &datasource.MappingConfiguration{
					Disabled: true,
				},
				DataSource: datasource.SourceConfig{
					Name: "HttpJsonDataSource",
					Config: toJSON(datasource.HttpJsonDataSourceConfig{
						Host: "localhost:9001",
						URL:  "/user/{{ .object.id }}/friends",
					}),
				},
			},
			{
				TypeName:  "query",
				FieldName: "user",
				DataSource: datasource.SourceConfig{
					Name: "GraphQLDataSource",
					Config: toJSON(datasource.GraphQLDataSourceConfig{
						Host: "localhost:8001",
						URL:  "/graphql",
					}),
				},
				Mapping: &datasource.MappingConfiguration{
					Path: "userPets",
				},
			},
		},
	}

	base, err := datasource.NewBaseDataSourcePlanner([]byte(schema), config, log.NoopLogger)
	if err != nil {
		b.Fatal(err)
	}

	planner := NewPlanner(base)
	var report operationreport.Report

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		planner.Plan(&op, &def, "", &report)
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

const countriesSchema = `directive @cacheControl(maxAge: Int, scope: CacheControlScope) on FIELD_DEFINITION | OBJECT | INTERFACE

scalar String
scalar ID
scalar Boolean

schema {
	query: Query
	mutation: Mutation
}

enum CacheControlScope {
  PUBLIC
  PRIVATE
}

type Continent {
  code: ID!
  name: String!
  countries: [Country!]!
}

input ContinentFilterInput {
  code: StringQueryOperatorInput
}

type Country {
  code: ID!
  name: String!
  native: String!
  phone: String!
  continent: Continent!
  capital: String
  currency: String
  languages: [Language!]!
  emoji: String!
  emojiU: String!
  states: [State!]!
}

input CountryFilterInput {
  code: StringQueryOperatorInput
  currency: StringQueryOperatorInput
  continent: StringQueryOperatorInput
}

type Language {
  code: ID!
  name: String
  native: String
  rtl: Boolean!
}

input LanguageFilterInput {
  code: StringQueryOperatorInput
}

type Query {
  continents(filter: ContinentFilterInput): [Continent!]!
  continent(code: ID!): Continent
  countries(filter: CountryFilterInput): [Country!]!
  country(code: ID!): Country
  languages(filter: LanguageFilterInput): [Language!]!
  language(code: ID!): Language
}

type Mutation {
	addLanguage(code: ID!, name: String!): Language!
}

type State {
  code: String
  name: String!
  country: Country!
}

input StringQueryOperatorInput {
  eq: String
  ne: String
  in: [String]
  nin: [String]
  regex: String
  glob: String
}

"""The Upload scalar type represents a file upload."""
scalar Upload`

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
							DataSource: &datasource.SchemaDataSource{
								SchemaBytes: schema,
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
									PathSelector: datasource.PathSelector{
										Path: "__schema",
									},
								},
								Fields: []Field{
									{
										Name: []byte("queryType"),
										Value: &Object{
											DataResolvingConfig: DataResolvingConfig{
												PathSelector: datasource.PathSelector{
													Path: "queryType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
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
												PathSelector: datasource.PathSelector{
													Path: "mutationType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
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
												PathSelector: datasource.PathSelector{
													Path: "subscriptionType",
												},
											},
											Fields: []Field{
												{
													Name: []byte("name"),
													Value: &Value{
														DataResolvingConfig: DataResolvingConfig{
															PathSelector: datasource.PathSelector{
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
												PathSelector: datasource.PathSelector{
													Path: "types",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("kind"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
																	Path: "fields",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
																					Path: "args",
																				},
																			},
																			Value: &Object{
																				Fields: []Field{
																					{
																						Name: []byte("name"),
																						Value: &Value{
																							DataResolvingConfig: DataResolvingConfig{
																								PathSelector: datasource.PathSelector{
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
																								PathSelector: datasource.PathSelector{
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
																								PathSelector: datasource.PathSelector{
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
																								PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
																	Path: "inputFields",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
																	Path: "enumValues",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
												PathSelector: datasource.PathSelector{
													Path: "directives",
												},
											},
											Value: &Object{
												Fields: []Field{
													{
														Name: []byte("name"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
																	Path: "args",
																},
															},
															Value: &Object{
																Fields: []Field{
																	{
																		Name: []byte("name"),
																		Value: &Value{
																			DataResolvingConfig: DataResolvingConfig{
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
																				PathSelector: datasource.PathSelector{
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
				PathSelector: datasource.PathSelector{
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
				PathSelector: datasource.PathSelector{
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
				PathSelector: datasource.PathSelector{
					Path: "ofType",
				},
			},
			Fields: []Field{
				{
					Name: []byte("kind"),
					Value: &Value{
						DataResolvingConfig: DataResolvingConfig{
							PathSelector: datasource.PathSelector{
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
							PathSelector: datasource.PathSelector{
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
							PathSelector: datasource.PathSelector{
								Path: "ofType",
							},
						},
						Fields: []Field{
							{
								Name: []byte("kind"),
								Value: &Value{
									DataResolvingConfig: DataResolvingConfig{
										PathSelector: datasource.PathSelector{
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
										PathSelector: datasource.PathSelector{
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
										PathSelector: datasource.PathSelector{
											Path: "ofType",
										},
									},
									Fields: []Field{
										{
											Name: []byte("kind"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
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
													PathSelector: datasource.PathSelector{
														Path: "ofType",
													},
												},
												Fields: []Field{
													{
														Name: []byte("kind"),
														Value: &Value{
															DataResolvingConfig: DataResolvingConfig{
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
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
																PathSelector: datasource.PathSelector{
																	Path: "ofType",
																},
															},
															Fields: []Field{
																{
																	Name: []byte("kind"),
																	Value: &Value{
																		DataResolvingConfig: DataResolvingConfig{
																			PathSelector: datasource.PathSelector{
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
																			PathSelector: datasource.PathSelector{
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
																			PathSelector: datasource.PathSelector{
																				Path: "ofType",
																			},
																		},
																		Fields: []Field{
																			{
																				Name: []byte("kind"),
																				Value: &Value{
																					DataResolvingConfig: DataResolvingConfig{
																						PathSelector: datasource.PathSelector{
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
																						PathSelector: datasource.PathSelector{
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
																						PathSelector: datasource.PathSelector{
																							Path: "ofType",
																						},
																					},
																					Fields: []Field{
																						{
																							Name: []byte("kind"),
																							Value: &Value{
																								DataResolvingConfig: DataResolvingConfig{
																									PathSelector: datasource.PathSelector{
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
																									PathSelector: datasource.PathSelector{
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
