package execution

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-test/deep"
	log "github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/pipeline/pkg/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

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
		if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
			t.Error(err)
		}

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
			assert.Equal(t, want, got)
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

	t.Run("GraphQLDataSource", run(GraphQLDataSourceSchema, `
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
									URL: "countries.trevorblades.com/",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("country"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("countries.trevorblades.com/"),
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
	t.Run("GraphQLDataSource mutation", run(GraphQLDataSourceSchema, `
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
								URL: "fakebook.com/",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultMutationTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("likePost"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("fakebook.com/"),
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
	t.Run("HTTPJSONDataSource", run(HTTPJSONDataSourceSchema, `
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
								URL: "jsonplaceholder.typicode.com/comments?postId={{ .object.id }}",
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
								URL: "httpbin.org/get",
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
								URL: "jsonplaceholder.typicode.com/posts/{{ .arguments.id }}",
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
												Name:  []byte("root_type_name"),
												Value: ast.DefaultQueryTypeName,
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("root_field_name"),
												Value: []byte("httpBinGet"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("httpbin.org/get"),
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
												Name:  []byte("root_type_name"),
												Value: ast.DefaultQueryTypeName,
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("root_field_name"),
												Value: []byte("post"),
											},
											&datasource.StaticVariableArgument{
												Name:  []byte("url"),
												Value: []byte("jsonplaceholder.typicode.com/posts/{{ .arguments.id }}"),
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
													Name:  []byte("root_type_name"),
													Value: []byte("JSONPlaceholderPost"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("root_field_name"),
													Value: []byte("comments"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("url"),
													Value: []byte("jsonplaceholder.typicode.com/comments?postId={{ .object.id }}"),
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
	t.Run("HTTPJSONDataSource withBody", run(HTTPJSONDataSourceSchema, `
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
								URL:    "httpbin.org/anything",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("withBody"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("httpbin.org/anything"),
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
	t.Run("HTTPJSONDataSource withPath", run(HTTPJSONDataSourceSchema, `
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
								URL: "httpbin.org/anything",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("withPath"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("httpbin.org/anything"),
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
	t.Run("HTTPJSONDataSource list withoutPath", run(HTTPJSONDataSourceSchema, `
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
								URL: "httpbin.org/anything",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("listItems"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("httpbin.org/anything"),
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
	t.Run("HTTPJSONDataSource list withPath", run(HTTPJSONDataSourceSchema, `
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
								URL: "httpbin.org/anything",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("listWithPath"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("httpbin.org/anything"),
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
	t.Run("HTTPJSONDataSource withHeaders", run(HTTPJSONDataSourceSchema, `
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
								URL: "httpbin.org/anything",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("withHeaders"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("httpbin.org/anything"),
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
	t.Run("StaticDataSource", run(staticDataSourceSchema, `
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
	t.Run("introspection type query", run(complexSchema, `
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
	t.Run("graphql resolver", run(complexSchema, `
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
								URL: "localhost:8001/graphql",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("user"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("localhost:8001/graphql"),
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
	t.Run("rest resolver", run(complexSchema, `
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
								URL: "localhost:9001/user/{{ .arguments.id }}",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("restUser"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("localhost:9001/user/{{ .arguments.id }}"),
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

	t.Run("graphql resolver with nested rest resolver", run(complexSchema, `
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
								URL: "localhost:8001/graphql",
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
								URL: "localhost:9001/user/{{ .object.id }}/friends",
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
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("user"),
									},
									&datasource.StaticVariableArgument{
										Name:  literal.URL,
										Value: []byte("localhost:8001/graphql"),
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
													Name:  []byte("root_type_name"),
													Value: []byte("User"),
												},
												&datasource.StaticVariableArgument{
													Name:  []byte("root_field_name"),
													Value: []byte("friends"),
												},
												&datasource.StaticVariableArgument{
													Name:  literal.URL,
													Value: []byte("localhost:9001/user/{{ .object.id }}/friends"),
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
	t.Run("introspection", run(complexSchema, `
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

	t.Run("http polling stream", run(HttpPollingStreamSchema, `
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
	t.Run("list filter first N", run(ListFilterFirstNSchema, `
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
	t.Run("stringPipeline", run(pipelineSchema, `
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
	t.Run("filePipeline", run(pipelineSchema, `
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
	schema := complexSchema
	def := unsafeparser.ParseGraphqlDocumentString(complexSchema)
	if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
		b.Fatal(err)
	}

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
						URL: "localhost:8001/graphql",
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
						URL: "localhost:9001/user/{{ .object.id }}/friends",
					}),
				},
			},
			{
				TypeName:  "query",
				FieldName: "user",
				DataSource: datasource.SourceConfig{
					Name: "GraphQLDataSource",
					Config: toJSON(datasource.GraphQLDataSourceConfig{
						URL: "localhost:8001/graphql",
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
