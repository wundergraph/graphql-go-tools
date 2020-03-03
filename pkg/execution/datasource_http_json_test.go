package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/execution/datasource"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/tidwall/gjson"
	"net/http"
	"net/http/httptest"
	"testing"
)

const httpJsonDataSourceSchema = `
schema {
    query: Query
}

type Query {
	simpleType: SimpleType
	unionType: UnionType
	interfaceType: InterfaceType
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
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "simpleType",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								data, _ := json.Marshal(HttpJsonDataSourceConfig{
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
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: abstractlogger.Noop{},
								},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&StaticVariableArgument{
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
													PathSelector: PathSelector{
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
													PathSelector: PathSelector{
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
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "unionType",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								defaultTypeName := "SuccessType"
								data, _ := json.Marshal(HttpJsonDataSourceConfig{
									Host:            "example.com",
									URL:             "/",
									DefaultTypeName: &defaultTypeName,
									StatusCodeTypeNameMappings: []StatusCodeTypeNameMapping{
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
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: abstractlogger.Noop{},
								},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&StaticVariableArgument{
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
													PathSelector: PathSelector{
														Path: "__typename",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("result"),
											Skip: &IfNotEqual{
												Left: &ObjectVariableArgument{
													PathSelector: PathSelector{
														Path: "__typename",
													},
												},
												Right: &StaticVariableArgument{
													Value: []byte("SuccessType"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "result",
													},
												},
												ValueType: StringValueType,
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
													Value: []byte("ErrorType"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
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
			base.config = PlannerConfiguration{
				TypeFieldConfigurations: []TypeFieldConfiguration{
					{
						TypeName:  "query",
						FieldName: "interfaceType",
						Mapping: &MappingConfiguration{
							Disabled: true,
						},
						DataSource: DataSourceConfig{
							Name: "HttpJsonDataSource",
							Config: func() []byte {
								defaultTypeName := "SuccessInterface"
								data, _ := json.Marshal(HttpJsonDataSourceConfig{
									Host:            "example.com",
									URL:             "/",
									DefaultTypeName: &defaultTypeName,
									StatusCodeTypeNameMappings: []StatusCodeTypeNameMapping{
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
			panicOnErr(base.RegisterDataSourcePlannerFactory("HttpJsonDataSource", HttpJsonDataSourcePlannerFactoryFactory{}))
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
								DataSource: &HttpJsonDataSource{
									log: abstractlogger.Noop{},
								},
								Args: []Argument{
									&StaticVariableArgument{
										Name:  []byte("host"),
										Value: []byte("example.com"),
									},
									&StaticVariableArgument{
										Name:  []byte("url"),
										Value: []byte("/"),
									},
									&StaticVariableArgument{
										Name:  []byte("method"),
										Value: []byte("GET"),
									},
									&StaticVariableArgument{
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
													PathSelector: PathSelector{
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
													PathSelector: PathSelector{
														Path: "name",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("successField"),
											Skip: &IfNotEqual{
												Left: &ObjectVariableArgument{
													PathSelector: PathSelector{
														Path: "__typename",
													},
												},
												Right: &StaticVariableArgument{
													Value: []byte("SuccessInterface"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
														Path: "successField",
													},
												},
												ValueType: StringValueType,
											},
										},
										{
											Name: []byte("errorField"),
											Skip: &IfNotEqual{
												Left: &ObjectVariableArgument{
													PathSelector: PathSelector{
														Path: "__typename",
													},
												},
												Right: &StaticVariableArgument{
													Value: []byte("ErrorInterface"),
												},
											},
											Value: &Value{
												DataResolvingConfig: DataResolvingConfig{
													PathSelector: PathSelector{
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
			source := &HttpJsonDataSource{
				log: abstractlogger.Noop{},
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
			source.Resolve(ctx, args, &buf)
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