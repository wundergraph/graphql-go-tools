package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/tidwall/gjson"

	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
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
	t.Run("simpleType", run(withBaseSchema(httpJsonDataSourceSchema), `
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
									Host: "example.com",
									URL:  "/",
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
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
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
	t.Run("list of strings", run(withBaseSchema(httpJsonDataSourceSchema), `
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
									Host: "example.com",
									URL:  "/",
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
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
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
	t.Run("list of objects", run(withBaseSchema(httpJsonDataSourceSchema), `
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
									Host: "example.com",
									URL:  "/",
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
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
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
	t.Run("unionType", run(withBaseSchema(httpJsonDataSourceSchema), `
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
									Host:            "example.com",
									URL:             "/",
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
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
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
	t.Run("interfaceType", run(withBaseSchema(httpJsonDataSourceSchema), `
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
									Host:            "example.com",
									URL:             "/",
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
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&datasource.StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
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
					Key:   []byte("host"),
					Value: []byte(fakeServer.URL),
				},
				{
					Key:   []byte("url"),
					Value: []byte("/"),
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
