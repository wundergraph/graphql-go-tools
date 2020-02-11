package execution

import (
	"bytes"
	"context"
	"fmt"
	"github.com/jensneuse/abstractlogger"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
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
        @HttpJsonDataSource(
            host: "example.com"
            url: "/"
        )
		@mapping(mode: NONE)
	unionType: UnionType
        @HttpJsonDataSource(
            host: "example.com"
            url: "/"
			defaultTypeName: "SuccessType"
			statusCodeTypeNameMappings: [
				{
					statusCode: 500
					typeName: "ErrorType"
				}
			]
        )
		@mapping(mode: NONE)
	interfaceType: InterfaceType
        @HttpJsonDataSource(
            host: "example.com"
            url: "/"
			defaultTypeName: "SuccessInterface"
			statusCodeTypeNameMappings: [
				{
					statusCode: 500
					typeName: "ErrorInterface"
				}
			]
        )
		@mapping(mode: NONE)
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

func makeHttpJsonDataSourceSchema() string {
	return withBaseSchema(httpJsonDataSourceSchema + httpJsonDataSourceBaseSchema)
}

func TestHttpJsonDataSourcePlanner_Plan(t *testing.T) {
	t.Run("simpleType", run(makeHttpJsonDataSourceSchema(), `
		query SimpleTypeQuery {
			simpleType {
				__typename
				scalarField
			}
		}
		`,
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("simpleType"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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
	t.Run("unionType", run(makeHttpJsonDataSourceSchema(), `
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
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("unionType"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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
	t.Run("interfaceType", run(makeHttpJsonDataSourceSchema(), `
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
		ResolverDefinitions{
			{
				TypeName:  literal.QUERY,
				FieldName: []byte("interfaceType"),
				DataSourcePlannerFactory: func() DataSourcePlanner {
					return &HttpJsonDataSourcePlanner{}
				},
			},
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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
												ValueType:StringValueType,
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

const httpJsonDataSourceBaseSchema = `
directive @HttpJsonDataSource (
    host: String!
    url: String!
    method: HTTP_METHOD = GET
    params: [Parameter]
	body: String
    defaultTypeName: String
    statusCodeTypeNameMappings: [StatusCodeTypeNameMapping]
) on FIELD_DEFINITION

input StatusCodeTypeNameMapping {
    statusCode: Int!
    typeName: String!
}

directive @mapping(
    mode: MAPPING_MODE! = PATH_SELECTOR
    pathSelector: String
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
}`
