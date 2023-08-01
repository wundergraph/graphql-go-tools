package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/wundergraph/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/pkg/execution/datasource"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

const httpJsonDataSourceSchema = `
schema {
    query: Query
}
type Query {
	simpleType: SimpleType
	unionType: UnionType
	interfaceType: InterfaceType
	listOfStrings: [String!]
	listOfObjects: [SimpleType]
}
type SimpleType {
	scalarField: String
}
union UnionType = SuccessType | ErrorType
type SuccessType {
	result: String
}
type ErrorType {
	message: String
}
interface InterfaceType {
	name: String!
}
type SuccessInterface implements InterfaceType {
	name: String!
	successField: String!
}
type ErrorInterface implements InterfaceType {
	name: String!
	errorField: String!
}
`

func TestHttpJsonDataSourcePlanner_Plan(t *testing.T) {
	t.Run("simpleType", run(httpJsonDataSourceSchema, `
		query SimpleTypeQuery {
			simpleType {
				__typename
				scalarField
			}
		}
		`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "simpleType",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
									URL: "example.com/",
									Method: func() *string {
										method := "GET"
										return &method
									}(),
									DefaultTypeName: func() *string {
										typeName := "SimpleType"
										return &typeName
									}(),
								})
								return data
							}(),
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
					Name: literal.DATA,
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "simpleType",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    abstractlogger.Noop{},
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("simpleType"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("example.com/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"SimpleType"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("simpleType"),
								HasResolvedData: true,
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
											Name: []byte("scalarField"),
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "scalarField",
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
	t.Run("list of strings", run(httpJsonDataSourceSchema, `
		query ListOfStrings {
			listOfStrings
		}
		`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listOfStrings",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
									URL: "example.com/",
									Method: func() *string {
										method := "GET"
										return &method
									}(),
								})
								return data
							}(),
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
					Name: literal.DATA,
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "listOfStrings",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    abstractlogger.Noop{},
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("listOfStrings"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("example.com/"),
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
								Name:            []byte("listOfStrings"),
								HasResolvedData: true,
								Value: &List{
									Value: &Value{
										ValueType: StringValueType,
									},
								},
							},
						},
					},
				},
			},
		},
	))
	t.Run("list of objects", run(httpJsonDataSourceSchema, `
		query ListOfObjects {
			listOfObjects {
				scalarField
			}
		}
		`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "listOfObjects",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
									URL: "example.com/",
									Method: func() *string {
										method := "GET"
										return &method
									}(),
									DefaultTypeName: func() *string {
										typeName := "SimpleType"
										return &typeName
									}(),
								})
								return data
							}(),
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
					Name: literal.DATA,
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "listOfObjects",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    abstractlogger.Noop{},
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("listOfObjects"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("example.com/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"defaultTypeName":"SimpleType"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("listOfObjects"),
								HasResolvedData: true,
								Value: &List{
									Value: &Object{
										Fields: []Field{
											{
												Name: []byte("scalarField"),
												Value: &Value{
													DataResolvingConfig: DataResolvingConfig{
														PathSelector: datasource.PathSelector{
															Path: "scalarField",
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
	t.Run("unionType", run(httpJsonDataSourceSchema, `
		query UnionTypeQuery {
			unionType {
				__typename
				... on SuccessType {
					result
				}
				... on ErrorType {
					message
				}
			}
		}
		`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "unionType",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								defaultTypeName := "SuccessType"
								data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
									URL:             "example.com/",
									DefaultTypeName: &defaultTypeName,
									StatusCodeTypeNameMappings: []datasource.StatusCodeTypeNameMapping{
										{
											StatusCode: 500,
											TypeName:   "ErrorType",
										},
									},
								})
								return data
							}(),
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
					Name: literal.DATA,
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "unionType",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    abstractlogger.Noop{},
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("unionType"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("example.com/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"500":"ErrorType","defaultTypeName":"SuccessType"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("unionType"),
								HasResolvedData: true,
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
											Name: []byte("result"),
											Skip: &IfNotEqual{
												Left: &datasource.ObjectVariableArgument{
													PathSelector: datasource.PathSelector{
														Path: "__typename",
													},
												},
												Right: &datasource.StaticVariableArgument{
													Value: []byte("SuccessType"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "result",
													},
												},
												ValueType: StringValueType,
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
													Value: []byte("ErrorType"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "message",
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
	t.Run("interfaceType", run(httpJsonDataSourceSchema, `
		query InterfaceTypeQuery {
			interfaceType {
				__typename
				name
				... on SuccessInterface {
					successField
				}
				... on ErrorInterface {
					errorField
				}
			}
		}
		`,
		func(base *datasource.BasePlanner) {
			base.Config = datasource.PlannerConfiguration{
				TypeFieldConfigurations: []datasource.TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "interfaceType",
						Mapping: &datasource.MappingConfiguration{
							Disabled: true,
						},
						DataSource: datasource.SourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								defaultTypeName := "SuccessInterface"
								data, _ := json.Marshal(datasource.HttpJsonDataSourceConfig{
									URL:             "example.com/",
									DefaultTypeName: &defaultTypeName,
									StatusCodeTypeNameMappings: []datasource.StatusCodeTypeNameMapping{
										{
											StatusCode: 500,
											TypeName:   "ErrorInterface",
										},
									},
								})
								return data
							}(),
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
					Name: literal.DATA,
					Value: &Object{
						Fetch: &SingleFetch{
							BufferName: "interfaceType",
							Source: &DataSourceInvocation{
								DataSource: &datasource.HttpJsonDataSource{
									Log:    abstractlogger.Noop{},
									Client: datasource.DefaultHttpClient(),
								},
								Args: []datasource.Argument{
									&datasource.StaticVariableArgument{
										Name:  []byte("root_type_name"),
										Value: ast.DefaultQueryTypeName,
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("root_field_name"),
										Value: []byte("interfaceType"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("example.com/"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("__typename"),
										Value: []byte(`{"500":"ErrorInterface","defaultTypeName":"SuccessInterface"}`),
									},
								},
							},
						},
						Fields: []Field{
							{
								Name:            []byte("interfaceType"),
								HasResolvedData: true,
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
											Name: []byte("successField"),
											Skip: &IfNotEqual{
												Left: &datasource.ObjectVariableArgument{
													PathSelector: datasource.PathSelector{
														Path: "__typename",
													},
												},
												Right: &datasource.StaticVariableArgument{
													Value: []byte("SuccessInterface"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "successField",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("errorField"),
											Skip: &IfNotEqual{
												Left: &datasource.ObjectVariableArgument{
													PathSelector: datasource.PathSelector{
														Path: "__typename",
													},
												},
												Right: &datasource.StaticVariableArgument{
													Value: []byte("ErrorInterface"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: datasource.PathSelector{
														Path: "errorField",
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
}

func TestHttpJsonDataSource_Resolve(t *testing.T) {

	test := func(serverStatusCode int, typeNameDefinition, wantTypeName string) func(t *testing.T) {
		return func(t *testing.T) {
			fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(serverStatusCode)
				_, _ = w.Write([]byte(`{"foo":"bar"}`))
			}))
			defer fakeServer.Close()
			ctx := Context{
				Context: context.Background(),
			}
			buf := bytes.Buffer{}
			source := &datasource.HttpJsonDataSource{
				Log:    abstractlogger.Noop{},
				Client: datasource.DefaultHttpClient(),
			}
			args := ResolvedArgs{
				{
					Key:   []byte("url"),
					Value: []byte(fakeServer.URL + "/"),
				},
				{
					Key:   []byte("method"),
					Value: []byte("GET"),
				},
				{
					Key:   []byte("__typename"),
					Value: []byte(typeNameDefinition),
				},
			}
			_, err := source.Resolve(ctx, args, &buf)
			if err != nil {
				t.Fatal(err)
			}
			result := gjson.GetBytes(buf.Bytes(), "__typename")
			gotTypeName := result.Str
			if gotTypeName != wantTypeName {
				panic(fmt.Errorf("want: %s, got: %s\n", wantTypeName, gotTypeName))
			}
		}
	}

	t.Run("typename selection on err", test(
		500,
		`{"500":"ErrorInterface","defaultTypeName":"SuccessInterface"}`,
		"ErrorInterface"))
	t.Run("typename selection on success using default", test(
		200,
		`{"500":"ErrorInterface","defaultTypeName":"SuccessInterface"}`,
		"SuccessInterface"))
	t.Run("typename selection on success using select", test(
		200,
		`{"500":"ErrorInterface","200":"AnotherSuccess","defaultTypeName":"SuccessInterface"}`,
		"AnotherSuccess"))
}

var httpJsonDataSourceName = "http_json"

func TestHttpJsonDataSource_WithPlanning(t *testing.T) {
	type testCase struct {
		definition            string
		operation             datasource.GraphqlRequest
		typeFieldConfigs      []datasource.TypeFieldConfiguration
		hooksFactory          func(t *testing.T) datasource.Hooks
		assertRequestBody     bool
		expectedRequestBodies []string
		upstreamResponses     []string
		expectedResponseBody  string
	}

	run := func(tc testCase) func(t *testing.T) {
		return func(t *testing.T) {
			upstreams := make([]*httptest.Server, len(tc.upstreamResponses))
			for i := 0; i < len(tc.upstreamResponses); i++ {
				if tc.assertRequestBody {
					require.Len(t, tc.expectedRequestBodies, len(tc.upstreamResponses))
				}

				var expectedRequestBody string
				if tc.assertRequestBody {
					expectedRequestBody = tc.expectedRequestBodies[i]
				}

				upstream := upstreamHttpJsonServer(t, tc.assertRequestBody, expectedRequestBody, tc.upstreamResponses[i])
				defer upstream.Close()

				upstreams[i] = upstream
			}

			var upstreamURLs []string
			for _, upstream := range upstreams {
				upstreamURLs = append(upstreamURLs, upstream.URL)
			}

			plannerConfig := createPlannerConfigToUpstream(t, upstreamURLs, http.MethodPost, tc.typeFieldConfigs)
			basePlanner, err := datasource.NewBaseDataSourcePlanner([]byte(tc.definition), plannerConfig, abstractlogger.NoopLogger)
			require.NoError(t, err)

			var hooks datasource.Hooks
			if tc.hooksFactory != nil {
				hooks = tc.hooksFactory(t)
			}

			err = basePlanner.RegisterDataSourcePlannerFactory(httpJsonDataSourceName, &datasource.HttpJsonDataSourcePlannerFactoryFactory{Hooks: hooks})
			require.NoError(t, err)

			definitionDocument := unsafeparser.ParseGraphqlDocumentString(tc.definition)
			operationDocument := unsafeparser.ParseGraphqlDocumentString(tc.operation.Query)

			var report operationreport.Report
			operationDocument.Input.Variables = tc.operation.Variables
			normalizer := astnormalization.NewNormalizer(true, true)
			normalizer.NormalizeOperation(&operationDocument, &definitionDocument, &report)
			require.False(t, report.HasErrors())

			tc.operation.Variables = operationDocument.Input.Variables

			planner := NewPlanner(basePlanner)
			plan := planner.Plan(&operationDocument, &definitionDocument, tc.operation.OperationName, &report)
			require.False(t, report.HasErrors())

			variables, extraArguments := VariablesFromJson(tc.operation.Variables, nil)
			executionContext := Context{
				Context:        context.Background(),
				Variables:      variables,
				ExtraArguments: extraArguments,
			}

			var buf bytes.Buffer
			executor := NewExecutor(nil)
			err = executor.Execute(executionContext, plan, &buf)
			require.NoError(t, err)

			assert.JSONEq(t, tc.expectedResponseBody, buf.String())
		}
	}

	t.Run("should execute hooks", run(
		testCase{
			definition: countriesSchema,
			operation: datasource.GraphqlRequest{
				OperationName: "",
				Variables:     nil,
				Query:         `{ country(code: "DE") { code name } }`,
			},
			typeFieldConfigs: []datasource.TypeFieldConfiguration{
				httpJsonTypeFieldConfigCountry,
			},
			hooksFactory: func(t *testing.T) datasource.Hooks {
				return datasource.Hooks{
					PreSendHttpHook: preSendHttpHookFunc(func(ctx datasource.HookContext, req *http.Request) {
						assert.Equal(t, ctx.TypeName, "Query")
						assert.Equal(t, ctx.FieldName, "country")
						assert.Regexp(t, `http://127.0.0.1:[0-9]+`, req.URL.String())
					}),
					PostReceiveHttpHook: postReceiveHttpHookFunc(func(ctx datasource.HookContext, resp *http.Response, body []byte) {
						assert.Equal(t, ctx.TypeName, "Query")
						assert.Equal(t, ctx.FieldName, "country")
						assert.Equal(t, 200, resp.StatusCode)
						assert.Equal(t, body, []byte(`{ "code": "DE", "name": "Germany" }`))
					}),
				}
			},
			assertRequestBody: false,
			upstreamResponses: []string{
				`{ "code": "DE", "name": "Germany" }`,
			},
			expectedResponseBody: `{ "data": { "country": { "code": "DE", "name": "Germany" } } }`,
		}),
	)
}

func upstreamHttpJsonServer(t *testing.T, assertRequestBody bool, expectedRequestBody string, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NotNil(t, r.Body)

		bodyBytes, err := ioutil.ReadAll(r.Body)
		require.NoError(t, err)

		if assertRequestBody {
			isEqual := assert.JSONEq(t, expectedRequestBody, string(bodyBytes))
			if !isEqual {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}

		_, err = w.Write([]byte(response))
		require.NoError(t, err)
	}))
}

var httpJsonTypeFieldConfigCountry = datasource.TypeFieldConfiguration{
	TypeName:  "Query",
	FieldName: "country",
	Mapping: &datasource.MappingConfiguration{
		Disabled: true,
		Path:     "country",
	},
	DataSource: datasource.SourceConfig{
		Name: httpJsonDataSourceName,
	},
}
