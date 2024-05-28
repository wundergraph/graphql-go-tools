package graphql_datasource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	. "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasourcetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/subscriptiontesting"
)

func mustSchema(t *testing.T, federationConfiguration *FederationConfiguration, schema string) *SchemaConfiguration {
	t.Helper()
	s, err := NewSchemaConfiguration(schema, federationConfiguration)
	require.NoError(t, err)
	return s
}

func mustCustomConfiguration(t *testing.T, input ConfigurationInput) Configuration {
	t.Helper()

	cfg, err := NewConfiguration(input)
	require.NoError(t, err)
	return cfg
}

func mustDataSourceConfiguration(t *testing.T, id string, metadata *plan.DataSourceMetadata, config Configuration) plan.DataSource {
	t.Helper()

	dsCfg, err := plan.NewDataSourceConfiguration[Configuration](id, &Factory[Configuration]{}, metadata, config)
	require.NoError(t, err)

	return dsCfg
}

func mustDataSourceConfigurationWithHttpClient(t *testing.T, id string, metadata *plan.DataSourceMetadata, config Configuration) plan.DataSource {
	t.Helper()

	dsCfg, err := plan.NewDataSourceConfiguration[Configuration](id, &Factory[Configuration]{httpClient: http.DefaultClient}, metadata, config)
	require.NoError(t, err)

	return dsCfg
}

func TestGraphQLDataSourceTypenames(t *testing.T) {
	t.Run("__typename on union", func(t *testing.T) {
		def := `
			schema {
				query: Query
			}
	
			type A {
				a: String
			}
	
			union U = A
	
			type Query {
				u: U
			}`

		t.Run("run", RunTest(
			def, `
			query TypenameOnUnion {
				u {
					__typename
				}
			}`,
			"TypenameOnUnion", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource:     &Source{},
								Input:          `{"method":"POST","url":"https://example.com/graphql","body":{"query":"{u {__typename}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("u"),
								Value: &resolve.Object{
									Path:     []string{"u"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												IsTypeName: true,
											},
										},
									},
								},
							},
						},
					},
				},
			}, plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"ds-id",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"u"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://example.com/graphql",
							},
							SchemaConfiguration: mustSchema(t, nil, def),
						}),
					),
				},
				DisableResolveFieldPositions: true,
			}))
	})
}

func TestGraphQLDataSource(t *testing.T) {
	t.Run("@removeNullVariables directive", func(t *testing.T) {
		// XXX: Directive needs to be explicitly declared
		definition := `
		directive @removeNullVariables on QUERY | MUTATION

		schema {
			query: Query
		}
		
		type Query {
			hero(a: String): String
		}`

		t.Run("@removeNullVariables directive", RunTest(definition, `
			query MyQuery($a: String) @removeNullVariables {
				hero(a: $a)
			}
			`, "MyQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource: &Source{},
								Input:      `{"method":"POST","url":"https://swapi.com/graphql","unnull_variables":true,"body":{"query":"query($a: String){hero(a: $a)}","variables":{"a":$$0$$}}}`,
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"a"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
									},
								),
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("hero"),
								Value: &resolve.String{
									Path:     []string{"hero"},
									Nullable: true,
								},
							},
						},
					},
				},
			}, plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"ds-id",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"hero"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://swapi.com/graphql",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
				},
				Fields: []plan.FieldConfiguration{
					{
						TypeName:  "Query",
						FieldName: "hero",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "a",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				DisableResolveFieldPositions: true,
			}))
	})

	t.Run("simple named Query", RunTest(starWarsSchema, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			hero {
				name
			}
			stringList
			nestedStringList
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$1$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("droid"),
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
								},
								{
									Name: []byte("friends"),
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
							},
						},
					},
					{
						Name: []byte("hero"),
						Value: &resolve.Object{
							Path:     []string{"hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
							},
						},
					},
					{
						Name: []byte("stringList"),
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						Name: []byte("nestedStringList"),
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"nestedStringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"droid", "hero", "stringList", "nestedStringList"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					SchemaConfiguration: mustSchema(t, nil, starWarsSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:              "Query",
				FieldName:             "stringList",
				DisableDefaultMapping: true,
			},
			{
				TypeName:  "Query",
				FieldName: "nestedStringList",
				Path:      []string{"nestedStringList"},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("simple named Query with field info", RunTest(starWarsSchema, `
		query MyQuery($id: ID!) {
			droid(id: $id){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			hero {
				name
			}
			stringList
			nestedStringList
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Info: &resolve.GraphQLResponseInfo{
				OperationType: ast.OperationTypeQuery,
			},
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$1$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					Info: &resolve.FetchInfo{
						OperationType: ast.OperationTypeQuery,
						DataSourceID:  "https://swapi.com",
						RootFields: []resolve.GraphCoordinate{
							{
								TypeName:  "Query",
								FieldName: "droid",
							},
							{
								TypeName:  "Query",
								FieldName: "hero",
							},
							{
								TypeName:  "Query",
								FieldName: "stringList",
							},
							{
								TypeName:  "Query",
								FieldName: "nestedStringList",
							},
						},
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("droid"),
						Info: &resolve.FieldInfo{
							Name:                "droid",
							ParentTypeNames:     []string{"Query"},
							ExactParentTypeName: "Query",
							NamedType:           "Droid",
							Source: resolve.TypeFieldSource{
								IDs: []string{"https://swapi.com"},
							},
						},
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Info: &resolve.FieldInfo{
										Name:                "name",
										ParentTypeNames:     []string{"Droid"},
										ExactParentTypeName: "Droid",
										NamedType:           "String",
										Source: resolve.TypeFieldSource{
											IDs: []string{"https://swapi.com"},
										},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
									Info: &resolve.FieldInfo{
										Name:                "name",
										ParentTypeNames:     []string{"Droid"},
										ExactParentTypeName: "Droid",
										NamedType:           "String",
										Source: resolve.TypeFieldSource{
											IDs: []string{"https://swapi.com"},
										},
									},
								},
								{
									Name: []byte("friends"),
									Info: &resolve.FieldInfo{
										Name:                "friends",
										ParentTypeNames:     []string{"Droid"},
										ExactParentTypeName: "Droid",
										NamedType:           "Character",
										Source: resolve.TypeFieldSource{
											IDs: []string{"https://swapi.com"},
										},
									},
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
													Info: &resolve.FieldInfo{
														Name:                "name",
														ParentTypeNames:     []string{"Character"},
														ExactParentTypeName: "Character",
														NamedType:           "String",
														Source: resolve.TypeFieldSource{
															IDs: []string{"https://swapi.com"},
														},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									Info: &resolve.FieldInfo{
										Name:                "primaryFunction",
										ParentTypeNames:     []string{"Droid"},
										ExactParentTypeName: "Droid",
										NamedType:           "String",
										Source: resolve.TypeFieldSource{
											IDs: []string{"https://swapi.com"},
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("hero"),
						Value: &resolve.Object{
							Path:     []string{"hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Info: &resolve.FieldInfo{
										Name:                "name",
										ParentTypeNames:     []string{"Character"},
										ExactParentTypeName: "Character",
										NamedType:           "String",
										Source: resolve.TypeFieldSource{
											IDs: []string{"https://swapi.com"},
										},
									},
								},
							},
						},
						Info: &resolve.FieldInfo{
							Name:                "hero",
							ParentTypeNames:     []string{"Query"},
							ExactParentTypeName: "Query",
							NamedType:           "Character",
							Source: resolve.TypeFieldSource{
								IDs: []string{"https://swapi.com"},
							},
						},
					},
					{
						Name: []byte("stringList"),
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
						Info: &resolve.FieldInfo{
							Name:                "stringList",
							ParentTypeNames:     []string{"Query"},
							ExactParentTypeName: "Query",
							NamedType:           "String",
							Source: resolve.TypeFieldSource{
								IDs: []string{"https://swapi.com"},
							},
						},
					},
					{
						Name: []byte("nestedStringList"),
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"nestedStringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
						Info: &resolve.FieldInfo{
							Name:                "nestedStringList",
							ParentTypeNames:     []string{"Query"},
							ExactParentTypeName: "Query",
							NamedType:           "String",
							Source: resolve.TypeFieldSource{
								IDs: []string{"https://swapi.com"},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		IncludeInfo: true,
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"https://swapi.com",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"droid", "hero", "stringList", "nestedStringList"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					SchemaConfiguration: mustSchema(t, nil, starWarsSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:              "Query",
				FieldName:             "stringList",
				DisableDefaultMapping: true,
			},
			{
				TypeName:  "Query",
				FieldName: "nestedStringList",
				Path:      []string{"nestedStringList"},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("selections on interface type", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("selections on interface type with on type condition", func(t *testing.T) {
		definition := `
			type Query {
			  thing: Thing
			}
			
			type Thing {
			  id: String!
			  abstractThing: AbstractThing
			}
			
			interface AbstractThing {
			  name: String
			}
			
			type ConcreteOne implements AbstractThing {
			  name: String
			}
			
			type ConcreteTwo implements AbstractThing {
			  name: String
			}`

		t.Run("run", RunTest(
			definition, `
			{
			  thing {
				id
				abstractThing {
				  ... on ConcreteOne {
					name
				  }
				}
			  }
			}`,
			"", &plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								DataSource:     &Source{},
								Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{thing {id abstractThing {__typename ... on ConcreteOne {name}}}}"}}`,
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("thing"),
								Value: &resolve.Object{
									Path:     []string{"thing"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.String{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("abstractThing"),
											Value: &resolve.Object{
												Path:     []string{"abstractThing"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("ConcreteOne")},
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
			}, plan.Configuration{
				DataSources: []plan.DataSource{

					mustDataSourceConfiguration(
						t,
						"ds-id",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"thing"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Thing",
									FieldNames: []string{"id", "abstractThing"},
								},
								{
									TypeName:   "AbstractThing",
									FieldNames: []string{"name"},
								},
								{
									TypeName:   "ConcreteOne",
									FieldNames: []string{"name"},
								},
								{
									TypeName:   "ConcreteTwo",
									FieldNames: []string{"name"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://swapi.com/graphql",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
				},
				Fields:                       []plan.FieldConfiguration{},
				DisableResolveFieldPositions: true,
			}))
	})

	t.Run("skip directive with variable", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				id
				displayName @skip(if: $skip)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($skip: Boolean!){user {id displayName @skip(if: $skip)}}","variables":{"skip":$$0$$}}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"skip"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
						),
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("skip directive on __typename", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				id
				displayName
				__typename @skip(if: $skip)
				tn2: __typename @include(if: $skip)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($skip: Boolean!){user {id displayName __typename @skip(if: $skip) tn2: __typename @include(if: $skip)}}","variables":{"skip":$$0$$}}}`,
						Variables: resolve.NewVariables(&resolve.ContextVariable{
							Path:     []string{"skip"},
							Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
						}),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
								},
								{
									Name: []byte("__typename"),
									Value: &resolve.String{
										Path:       []string{"__typename"},
										IsTypeName: true,
									},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
								},
								{
									Name: []byte("tn2"),
									Value: &resolve.String{
										Path:       []string{"tn2"},
										IsTypeName: true,
									},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "skip",
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("skip directive on an inline fragment", RunTest(interfaceSelectionSchema, `
		query MyQuery ($skip: Boolean!) {
			user {
				... @skip(if: $skip) {
					id
					displayName
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($skip: Boolean!){user {... @skip(if: $skip){id displayName}}}","variables":{"skip":$$0$$}}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"skip"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
						),
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									OnTypeNames:          [][]byte{[]byte("RegisteredUser")},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									OnTypeNames:          [][]byte{[]byte("RegisteredUser")},
									SkipDirectiveDefined: true,
									SkipVariableName:     "skip",
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("include directive on an inline fragment", RunTest(interfaceSelectionSchema, `
		query MyQuery ($include: Boolean!) {
			user {
				... @include(if: $include) {
					id
					displayName
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($include: Boolean!){user {... @include(if: $include){id displayName}}}","variables":{"include":$$0$$}}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"include"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
						),
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
									OnTypeNames:             [][]byte{[]byte("RegisteredUser")},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									OnTypeNames:             [][]byte{[]byte("RegisteredUser")},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("skip directive with inline value true", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @skip(if: true)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("skip directive with inline value false", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @skip(if: false)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("include directive with variable", RunTest(interfaceSelectionSchema, `
		query MyQuery ($include: Boolean!) {
			user {
				id
				displayName @include(if: $include)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($include: Boolean!){user {id displayName @include(if: $include)}}","variables":{"include":$$0$$}}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"include"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
							},
						),
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
									IncludeDirectiveDefined: true,
									IncludeVariableName:     "include",
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("include directive with inline value true", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @include(if: true)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))
	t.Run("include directive with inline value false", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName @include(if: false)
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("selections on interface type with object type interface", RunTest(interfaceSelectionSchema, `
		query MyQuery {
			user {
				id
				displayName
				... on RegisteredUser {
					hasVerifiedEmail
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource:     &Source{},
						Input:          `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"{user {id displayName __typename ... on RegisteredUser {hasVerifiedEmail}}}"}}`,
						PostProcessing: DefaultPostProcessingConfiguration,
					},
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("displayName"),
									Value: &resolve.String{
										Path: []string{"displayName"},
									},
								},
								{
									Name: []byte("hasVerifiedEmail"),
									Value: &resolve.Boolean{
										Path: []string{"hasVerifiedEmail"},
									},
									OnTypeNames: [][]byte{[]byte("RegisteredUser")},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"id", "displayName", "isLoggedIn"},
						},
						{
							TypeName:   "RegisteredUser",
							FieldNames: []string{"id", "displayName", "isLoggedIn", "hasVerifiedEmail"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, interfaceSelectionSchema),
				}),
			),
		},
		Fields:                       []plan.FieldConfiguration{},
		DisableResolveFieldPositions: true,
	}))

	t.Run("variable at top level and recursively", RunTest(variableSchema, `
		query MyQuery($name: String!){
            user(name: $name){
                normalized(data: {name: $name})
            }
        }
    `, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($name: String!){user(name: $name){normalized(data: {name: $name})}}","variables":{"name":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"name"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("user"),
						Value: &resolve.Object{
							Path:     []string{"user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("normalized"),
									Value: &resolve.String{
										Path: []string{"normalized"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
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
							FieldNames: []string{"normalized"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, variableSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "user",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "User",
				FieldName: "normalized",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "data",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("exported ID scalar field", RunTest(starWarsSchemaWithExportDirective, `
			query MyQuery($heroId: ID!){
				droid(id: $heroId){
					name
				}
				hero {
					id @export(as: "heroId")
				}
			}
			`, "MyQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							DataSource: &Source{},
							Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($heroId: ID!){droid(id: $heroId){name} hero {id}}","variables":{"heroId":$$0$$}}}`,
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"heroId"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("droid"),
							Value: &resolve.Object{
								Path:     []string{"droid"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
						{
							Name: []byte("hero"),
							Value: &resolve.Object{
								Path:     []string{"hero"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
											Export: &resolve.FieldExport{
												Path:     []string{"heroId"},
												AsString: true,
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"droid", "hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"id"},
							},
							{
								TypeName:   "Droid",
								FieldNames: []string{"name"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://swapi.com/graphql",
						},
						SchemaConfiguration: mustSchema(t, nil, starWarsSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "droid",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("exported string field", RunTest(starWarsSchemaWithExportDirective, `
		query MyQuery($id: ID! $heroName: String!){
			droid(id: $id){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			hero {
				name @export(as: "heroName")
			}
			search(name: $heroName) {
				... on Droid {
					primaryFunction
				}
			}
			stringList
			nestedStringList
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$2$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!, $heroName: String!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} search(name: $heroName){__typename ... on Droid {primaryFunction}} stringList nestedStringList}","variables":{"heroName":$$1$$,"id":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"heroName"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("droid"),
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
								},
								{
									Name: []byte("friends"),
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
							},
						},
					},
					{
						Name: []byte("hero"),
						Value: &resolve.Object{
							Path:     []string{"hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
										Export: &resolve.FieldExport{
											Path:     []string{"heroName"},
											AsString: true,
										},
									},
								},
							},
						},
					},
					{
						Name: []byte("search"),
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"search"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									OnTypeNames: [][]byte{[]byte("Droid")},
								},
							},
						},
					},
					{
						Name: []byte("stringList"),
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						Name: []byte("nestedStringList"),
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"nestedStringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"droid", "hero", "stringList", "nestedStringList", "search"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					SchemaConfiguration: mustSchema(t, nil, starWarsSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:              "Query",
				FieldName:             "stringList",
				DisableDefaultMapping: true,
			},
			{
				TypeName:  "Query",
				FieldName: "nestedStringList",
				Path:      []string{"nestedStringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("Query with renamed root fields", RunTest(renamedStarWarsSchema, `
		query MyQuery($id: ID! $input: SearchInput_api! @api_onVariable $options: JSON_api) @otherapi_undefined @api_onOperation {
			api_droid(id: $id){
				name @api_format
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			api_hero {
				name
				... on Human_api {
					height
				}
			}
			api_stringList
			renamed: api_nestedStringList
			api_search(name: "r2d2") {
				... on Droid_api {
					primaryFunction
				}
			}
			api_searchWithInput(input: $input) {
				... on Droid_api {
					primaryFunction
				}
			}
			withOptions: api_searchWithInput(input: {
				options: $options
			}) {
				... on Droid_api {
					primaryFunction
				}
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$4$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!, $a: String! @onVariable, $input: SearchInput!, $options: JSON)@onOperation {api_droid: droid(id: $id){name @format aliased: name friends {name} primaryFunction} api_hero: hero {name __typename ... on Human {height}} api_stringList: stringList renamed: nestedStringList api_search: search(name: $a){__typename ... on Droid {primaryFunction}} api_searchWithInput: searchWithInput(input: $input){__typename ... on Droid {primaryFunction}} withOptions: searchWithInput(input: {options: $options}){__typename ... on Droid {primaryFunction}}}","variables":{"options":$$3$$,"input":$$2$$,"a":$$1$$,"id":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"a"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"input"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["object"],"properties":{"name":{"type":["string","null"]},"options":{}},"additionalProperties":false}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"options"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{}`),
							},
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("api_droid"),
						Value: &resolve.Object{
							Path:     []string{"api_droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
								},
								{
									Name: []byte("friends"),
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
							},
						},
					},
					{
						Name: []byte("api_hero"),
						Value: &resolve.Object{
							Path:     []string{"api_hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("height"),
									Value: &resolve.String{
										Path: []string{"height"},
									},
									OnTypeNames: [][]byte{[]byte("Human")},
								},
							},
						},
					},
					{
						Name: []byte("api_stringList"),
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"api_stringList"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						Name: []byte("renamed"),
						Value: &resolve.Array{
							Nullable: true,
							Path:     []string{"renamed"},
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						Name: []byte("api_search"),
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"api_search"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									OnTypeNames: [][]byte{[]byte("Droid")},
								},
							},
						},
					},
					{
						Name: []byte("api_searchWithInput"),
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"api_searchWithInput"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									OnTypeNames: [][]byte{[]byte("Droid")},
								},
							},
						},
					},
					{
						Name: []byte("withOptions"),
						Value: &resolve.Object{
							Nullable: true,
							Path:     []string{"withOptions"},
							Fields: []*resolve.Field{
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
									OnTypeNames: [][]byte{[]byte("Droid")},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"api_droid", "api_hero", "api_stringList", "api_nestedStringList", "api_search", "api_searchWithInput"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character_api",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human_api",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid_api",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
						{
							TypeName:   "SearchResult_api",
							FieldNames: []string{"name", "height", "primaryFunction", "friends"},
						},
					},
					Directives: plan.NewDirectiveConfigurations([]plan.DirectiveConfiguration{
						{
							DirectiveName: "api_format",
							RenameTo:      "format",
						},
						{
							DirectiveName: "api_onOperation",
							RenameTo:      "onOperation",
						},
						{
							DirectiveName: "api_onVariable",
							RenameTo:      "onVariable",
						},
					}),
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					SchemaConfiguration: mustSchema(t, nil, starWarsSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "api_droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
				Path: []string{"droid"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_hero",
				Path:      []string{"hero"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_stringList",
				Path:      []string{"stringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_nestedStringList",
				Path:      []string{"nestedStringList"},
			},
			{
				TypeName:  "Query",
				FieldName: "api_search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "name",
						SourceType:   plan.FieldArgumentSource,
						SourcePath:   []string{"name"},
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "api_searchWithInput",
				Path:      []string{"searchWithInput"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "input",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		Types: []plan.TypeConfiguration{
			{
				TypeName: "Human_api",
				RenameTo: "Human",
			},
			{
				TypeName: "Droid_api",
				RenameTo: "Droid",
			},
			{
				TypeName: "SearchResult_api",
				RenameTo: "SearchResult",
			},
			{
				TypeName: "SearchInput_api",
				RenameTo: "SearchInput",
			},
			{
				TypeName: "JSON_api",
				RenameTo: "JSON",
			},
		},
		DisableResolveFieldPositions: true,
	}, WithSkipReason("Renaming is broken")))

	t.Run("Query with array input", RunTest(subgraphTestSchema, `
		query($representations: [_Any!]!) {
			_entities(representations: $representations){
				... on Product {
					reviews {
						body 
						author {
							username 
							id
						}
					}
				}
			}
		}
	`, "", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://subgraph-reviews/query","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {reviews {body author {username id}}}}}","variables":{"representations":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"representations"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["array"],"items":{"type":["object"],"additionalProperties":true}}`),
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("_entities"),
						Value: &resolve.Array{
							Path:     []string{"_entities"},
							Nullable: false,
							Item: &resolve.Object{
								Nullable: true,
								Path:     nil,
								Fields: []*resolve.Field{
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Path:     nil,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path:     []string{"body"},
															Nullable: false,
														},
													},
													{
														Name: []byte("author"),
														Value: &resolve.Object{
															Nullable: false,
															Path:     []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path:     []string{"username"},
																		Nullable: false,
																	},
																},
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path:     []string{"id"},
																		Nullable: false,
																	},
																},
															},
														},
													},
												},
											},
										},
										OnTypeNames: [][]byte{[]byte("Product")},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"_entities", "_service"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "_Service",
							FieldNames: []string{"sdl"},
						},
						{
							TypeName:   "Entity",
							FieldNames: []string{"findProductByUpc", "findUserByID"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "reviews"},
						},
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "author", "product"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username", "reviews"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://subgraph-reviews/query",
					},
					SchemaConfiguration: mustSchema(t, nil, subgraphTestSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "_entities",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "representations",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Entity",
				FieldName: "findProductByUpc",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "upc",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Entity",
				FieldName: "findUserByID",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("Query with ID array input", func(t *testing.T) {
		t.Run("run", runTestOnTestDefinition(t, `
		query Droids($droidIDs: [ID!]!) {
			droids(ids: $droidIDs) {
				name
				primaryFunction
			}
		}`, "Droids",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($droidIDs: [ID!]!){droids(ids: $droidIDs){name primaryFunction}}","variables":{"droidIDs":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"droidIDs"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["array"],"items":{"type":["string","integer"]}}`),
									},
								),
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("droids"),
								Value: &resolve.Array{
									Path:     []string{"droids"},
									Nullable: true,
									Item: &resolve.Object{
										Nullable: true,
										Path:     nil,
										Fields: []*resolve.Field{
											{
												Name: []byte("name"),
												Value: &resolve.String{
													Path:     []string{"name"},
													Nullable: false,
												},
											},
											{
												Name: []byte("primaryFunction"),
												Value: &resolve.String{
													Path:     []string{"primaryFunction"},
													Nullable: false,
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
	})

	t.Run("Query with ID input", func(t *testing.T) {
		t.Run("run", runTestOnTestDefinition(t, `
		query Droid($droidID: ID!) {
			droid(id: $droidID) {
				name
				primaryFunction
			}
		}`, "Droid",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($droidID: ID!){droid(id: $droidID){name primaryFunction}}","variables":{"droidID":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"droidID"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
									},
								),
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("droid"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"droid"},
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
										{
											Name: []byte("primaryFunction"),
											Value: &resolve.String{
												Path:     []string{"primaryFunction"},
												Nullable: false,
											},
										},
									},
								},
							},
						},
					},
				},
			}))
	})

	t.Run("Query with Date input aka scalar", func(t *testing.T) {
		t.Run("run", runTestOnTestDefinition(t, `
		query HeroByBirthdate($birthdate: Date!) {
			heroByBirthdate(birthdate: $birthdate) {
				name
			}
		}`, "HeroByBirthdate",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($birthdate: Date!){heroByBirthdate(birthdate: $birthdate){name}}","variables":{"birthdate":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"birthdate"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{}`),
									},
								),
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("heroByBirthdate"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"heroByBirthdate"},
									Fields: []*resolve.Field{
										{
											Name: []byte("name"),
											Value: &resolve.String{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
									},
								},
							},
						},
					},
				},
			}))
	})

	t.Run("simple mutation", RunTest(`
		type Mutation {
			addFriend(name: String!):Friend!
		}
		type Friend {
			id: ID!
			name: String!
		}
	`,
		`mutation AddFriend($name: String!){ addFriend(name: $name){ id name } }`,
		"AddFriend",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"name"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("addFriend"),
							Value: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path: []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Mutation",
								FieldNames: []string{"addFriend"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Friend",
								FieldNames: []string{"id", "name"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://service.one",
						},
						SchemaConfiguration: mustSchema(t, nil, `
							type Mutation {
								addFriend(name: String!):Friend!
							}
							type Friend {
								id: ID!
								name: String!
							}
						`),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:              "Mutation",
					FieldName:             "addFriend",
					DisableDefaultMapping: true,
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "name",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))

	t.Run("nested resolvers of same upstream", RunTest(`
		type Query {
			foo(bar: String):Baz
		}
		type Baz {
			bar(bal: String):String
		}
		`,
		`
		query NestedQuery {
			foo(bar: "baz") {
				bar(bal: "bat")
			}
		}
		`,
		"NestedQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"https://foo.service","body":{"query":"query($a: String, $b: String){foo(bar: $a){bar(bal: $b)}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"foo"},
								Fields: []*resolve.Field{
									{
										Name: []byte("bar"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"bar"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"foo"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Baz",
								FieldNames: []string{"bar"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://foo.service",
						},
						SchemaConfiguration: mustSchema(t, nil, `
							type Query {
								foo(bar: String):Baz
							}
							type Baz {
								bar(bal: String):String
							}
						`),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "foo",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "bar",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Baz",
					FieldName: "bar",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "bal",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))

	t.Run("same upstream with alias in query", RunTest(
		countriesSchema,
		`
		query QueryWithAlias {
			country(code: "AD") {
				name
			}
			alias: country(code: "AE") {
				name
            }
		}
		`,
		"QueryWithAlias",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} alias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("country"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"country"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
									},
								},
							},
						},
						{
							Name: []byte("alias"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"alias"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"country", "countryAlias"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Country",
								FieldNames: []string{"name", "code"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://countries.service",
						},
						SchemaConfiguration: mustSchema(t, nil, countriesSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "country",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "countryAlias",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))

	t.Run("same upstream with alias in schema", RunTest(
		countriesSchema,
		`
		query QueryWithSchemaAlias {
			country(code: "AD") {
				name
			}
			countryAlias(code: "AE") {
				name
            }
		}
		`,
		"QueryWithSchemaAlias",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} countryAlias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"b"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("country"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"country"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
									},
								},
							},
						},
						{
							Name: []byte("countryAlias"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"countryAlias"},
								Fields: []*resolve.Field{
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Nullable: false,
											Path:     []string{"name"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"country", "countryAlias"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Country",
								FieldNames: []string{"name", "code"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://countries.service",
						},
						SchemaConfiguration: mustSchema(t, nil, countriesSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "country",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "countryAlias",
					Path:      []string{"country"},
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "code",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))

	t.Run("nested graphql engines", func(t *testing.T) {
		definition :=
			`
			type Query {
				serviceOne(serviceOneArg: String): ServiceOneResponse
				anotherServiceOne(anotherServiceOneArg: Int): ServiceOneResponse
				reusingServiceOne(reusingServiceOneArg: String): ServiceOneResponse
				serviceTwo(serviceTwoArg: Boolean): ServiceTwoResponse
				secondServiceTwo(secondServiceTwoArg: Float): ServiceTwoResponse
			}
			type ServiceOneResponse {
				fieldOne: String!
				countries: [Country!]!
			}
			type ServiceTwoResponse {
				fieldTwo: String
				serviceOneField: String
				serviceOneResponse: ServiceOneResponse
			}
			type Country {
				name: String!
			}
		`
		t.Run("nested graphql engines", RunTest(definition, `
		query NestedQuery ($firstArg: String, $secondArg: Boolean, $thirdArg: Int, $fourthArg: Float){
			serviceOne(serviceOneArg: $firstArg) {
				fieldOne
				countries {
					name
				}
			}
			serviceTwo(serviceTwoArg: $secondArg){
				fieldTwo
				serviceOneResponse {
					fieldOne
				}
			}
			anotherServiceOne(anotherServiceOneArg: $thirdArg){
				fieldOne
			}
			secondServiceTwo(secondServiceTwoArg: $fourthArg){
				fieldTwo
				serviceOneField
			}
			reusingServiceOne(reusingServiceOneArg: $firstArg){
				fieldOne
			}
		}
	`, "NestedQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.ParallelFetch{
							Fetches: []resolve.Fetch{
								&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 0,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
										DataSource: &Source{},
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"firstArg"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"thirdArg"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["integer","null"]}`),
											},
										),
										PostProcessing: DefaultPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								&resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID: 2,
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input:      `{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
										DataSource: &Source{},
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"secondArg"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"fourthArg"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["number","null"]}`),
											},
										),
										PostProcessing: DefaultPostProcessingConfiguration,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
							},
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("serviceOne"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"serviceOne"},
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 1,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											DataSource:     &Source{},
											Input:          `{"method":"POST","url":"https://country.service","body":{"query":"{countries {name}}"}}`,
											PostProcessing: DefaultPostProcessingConfiguration,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},

									Fields: []*resolve.Field{
										{
											Name: []byte("fieldOne"),
											Value: &resolve.String{
												Path: []string{"fieldOne"},
											},
										},
										{
											Name: []byte("countries"),
											Value: &resolve.Array{
												Path: []string{"countries"},
												Item: &resolve.Object{
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Value: &resolve.String{
																Path: []string{"name"},
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
								Name: []byte("serviceTwo"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"serviceTwo"},
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID: 3,
										},
										FetchConfiguration: resolve.FetchConfiguration{
											DataSource: &Source{},
											Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOneResponse: serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":$$0$$}}}`,
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path:     []string{"serviceOneField"},
													Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
												},
											),
											PostProcessing: DefaultPostProcessingConfiguration,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Fields: []*resolve.Field{
										{
											Name: []byte("fieldTwo"),
											Value: &resolve.String{
												Nullable: true,
												Path:     []string{"fieldTwo"},
											},
										},
										{
											Name: []byte("serviceOneResponse"),
											Value: &resolve.Object{
												Nullable: true,
												Path:     []string{"serviceOneResponse"},
												Fields: []*resolve.Field{
													{
														Name: []byte("fieldOne"),
														Value: &resolve.String{
															Path: []string{"fieldOne"},
														},
													},
												},
											},
										},
									},
								},
							},
							{
								Name: []byte("anotherServiceOne"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"anotherServiceOne"},
									Fields: []*resolve.Field{
										{
											Name: []byte("fieldOne"),
											Value: &resolve.String{
												Path: []string{"fieldOne"},
											},
										},
									},
								},
							},
							{
								Name: []byte("secondServiceTwo"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"secondServiceTwo"},
									Fields: []*resolve.Field{
										{
											Name: []byte("fieldTwo"),
											Value: &resolve.String{
												Path:     []string{"fieldTwo"},
												Nullable: true,
											},
										},
										{
											Name: []byte("serviceOneField"),
											Value: &resolve.String{
												Path:     []string{"serviceOneField"},
												Nullable: true,
											},
										},
									},
								},
							},
							{
								Name: []byte("reusingServiceOne"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"reusingServiceOne"},
									Fields: []*resolve.Field{
										{
											Name: []byte("fieldOne"),
											Value: &resolve.String{
												Path: []string{"fieldOne"},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"ds-id-1",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"serviceOne", "anotherServiceOne", "reusingServiceOne"},
								},
								{
									TypeName:   "ServiceTwoResponse",
									FieldNames: []string{"serviceOneResponse"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "ServiceOneResponse",
									FieldNames: []string{"fieldOne"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://service.one",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
					mustDataSourceConfiguration(
						t,
						"ds-id-2",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Query",
									FieldNames: []string{"serviceTwo", "secondServiceTwo"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "ServiceTwoResponse",
									FieldNames: []string{"fieldTwo", "serviceOneField"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://service.two",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
					mustDataSourceConfiguration(
						t,
						"ds-id-3",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "ServiceOneResponse",
									FieldNames: []string{"countries"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "Country",
									FieldNames: []string{"name"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://country.service",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
				},
				Fields: []plan.FieldConfiguration{
					{
						TypeName:  "ServiceTwoResponse",
						FieldName: "serviceOneResponse",
						Path:      []string{"serviceOne"},
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "serviceOneArg",
								SourceType: plan.ObjectFieldSource,
								SourcePath: []string{"serviceOneField"},
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "serviceTwo",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "serviceTwoArg",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "secondServiceTwo",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "secondServiceTwoArg",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "serviceOne",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "serviceOneArg",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "reusingServiceOne",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "reusingServiceOneArg",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
					{
						TypeName:  "Query",
						FieldName: "anotherServiceOne",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "anotherServiceOneArg",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				DisableResolveFieldPositions: true,
			},
			WithMultiFetchPostProcessor(),
		))
	})

	t.Run("mutation with variables in array object argument", RunTest(
		todoSchema,
		`mutation AddTask($title: String!, $completed: Boolean!, $name: String! @fromClaim(name: "sub")) {
					  addTask(input: [{titleSets: [[$title]], completed: $completed, user: {name: $name}}]){
						task {
						  id
						  title
						  completed
						}
					  }
					}`,
		"AddTask",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"https://graphql.service","body":{"query":"mutation($title: String!, $completed: Boolean!, $name: String!){addTask(input: [{titleSets: [[$title]],completed: $completed,user: {name: $name}}]){task {id title completed}}}","variables":{"name":$$2$$,"completed":$$1$$,"title":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"title"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"completed"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"name"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("addTask"),
							Value: &resolve.Object{
								Path:     []string{"addTask"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("task"),
										Value: &resolve.Array{
											Nullable: true,
											Path:     []string{"task"},
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path: []string{"id"},
														},
													},
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
														},
													},
													{
														Name: []byte("completed"),
														Value: &resolve.Boolean{
															Path: []string{"completed"},
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Mutation",
								FieldNames: []string{"addTask"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "AddTaskPayload",
								FieldNames: []string{"task"},
							},
							{
								TypeName:   "Task",
								FieldNames: []string{"id", "title", "completed"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "https://graphql.service",
						},
						SchemaConfiguration: mustSchema(t, nil, todoSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "addTask",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
	))

	t.Run("inline object value with arguments", func(t *testing.T) {
		definition := `
			schema {
				mutation: Mutation
			}
			type Mutation {
				createUser(input: CreateUserInput!): CreateUser
			}
			input CreateUserInput {
				user: UserInput
			}
			input UserInput {
				id: String
				username: String
			}
			type CreateUser {
				user: User
			}
			type User {
				id: String
				username: String
				createdDate: String
			}
			directive @fromClaim(name: String) on VARIABLE_DEFINITION
			`

		t.Run("inline object value with arguments", RunTest(definition, `
			mutation Register($name: String $id: String @fromClaim(name: "sub")) {
			  createUser(input: {user: {id: $id username: $name}}){
				user {
				  id
				  username
				  createdDate
				}
			  }
			}`,
			"Register",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:      `{"method":"POST","url":"https://user.service","body":{"query":"mutation($id: String, $name: String){createUser(input: {user: {id: $id,username: $name}}){user {id username createdDate}}}","variables":{"name":$$1$$,"id":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:     []string{"id"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
									},
									&resolve.ContextVariable{
										Path:     []string{"name"},
										Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
									},
								),
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("createUser"),
								Value: &resolve.Object{
									Nullable: true,
									Path:     []string{"createUser"},
									Fields: []*resolve.Field{
										{
											Name: []byte("user"),
											Value: &resolve.Object{
												Path:     []string{"user"},
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("id"),
														Value: &resolve.String{
															Path:     []string{"id"},
															Nullable: true,
														},
													},
													{
														Name: []byte("username"),
														Value: &resolve.String{
															Path:     []string{"username"},
															Nullable: true,
														},
													},
													{
														Name: []byte("createdDate"),
														Value: &resolve.String{
															Path:     []string{"createdDate"},
															Nullable: true,
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
			plan.Configuration{
				DataSources: []plan.DataSource{
					mustDataSourceConfiguration(
						t,
						"ds-id",
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{
									TypeName:   "Mutation",
									FieldNames: []string{"createUser"},
								},
							},
							ChildNodes: []plan.TypeField{
								{
									TypeName:   "CreateUser",
									FieldNames: []string{"user"},
								},
								{
									TypeName:   "User",
									FieldNames: []string{"id", "username", "createdDate"},
								},
							},
						},
						mustCustomConfiguration(t, ConfigurationInput{
							Fetch: &FetchConfiguration{
								URL: "https://user.service",
							},
							SchemaConfiguration: mustSchema(t, nil, definition),
						}),
					),
				},
				Fields: []plan.FieldConfiguration{
					{
						TypeName:  "Mutation",
						FieldName: "createUser",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:       "input",
								SourceType: plan.FieldArgumentSource,
							},
						},
					},
				},
				DisableResolveFieldPositions: true,
			},
		))
	})

	t.Run("mutation with union response", RunTest(wgSchema, `
		mutation CreateNamespace($name: String! $personal: Boolean!) {
			__typename
			namespaceCreate(input: {name: $name, personal: $personal}){
				__typename
				... on NamespaceCreated {
					namespace {
						id
						name
					}
				}
				... on Error {
					code
					message
				}
			}
		}`, "CreateNamespace",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://api.com","body":{"query":"mutation($name: String!, $personal: Boolean!){__typename namespaceCreate(input: {name: $name,personal: $personal}){__typename ... on NamespaceCreated {namespace {id name}} ... on Error {code message}}}","variables":{"personal":$$1$$,"name":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"name"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"personal"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__typename"),
							Value: &resolve.String{
								Path:       []string{"__typename"},
								Nullable:   false,
								IsTypeName: true,
							},
						},
						{
							Name: []byte("namespaceCreate"),
							Value: &resolve.Object{
								Path: []string{"namespaceCreate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("__typename"),
										Value: &resolve.String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
									{
										OnTypeNames: [][]byte{[]byte("NamespaceCreated")},
										Name:        []byte("namespace"),
										Value: &resolve.Object{
											Path: []string{"namespace"},
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: false,
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: false,
													},
												},
											},
										},
									},
									{
										OnTypeNames: [][]byte{[]byte("Error")},
										Name:        []byte("code"),
										Value: &resolve.String{
											Path: []string{"code"},
										},
									},
									{
										OnTypeNames: [][]byte{[]byte("Error")},
										Name:        []byte("message"),
										Value: &resolve.String{
											Path: []string{"message"},
										},
									},
								},
							},
						},
					},
				},
			},
		}, plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName: "Mutation",
								FieldNames: []string{
									"namespaceCreate",
								},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName: "NamespaceCreated",
								FieldNames: []string{
									"namespace",
								},
							},
							{
								TypeName:   "Namespace",
								FieldNames: []string{"id", "name"},
							},
							{
								TypeName:   "Error",
								FieldNames: []string{"code", "message"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL:    "http://api.com",
							Method: "POST",
						},
						Subscription: &SubscriptionConfiguration{
							URL: "ws://api.com",
						},
						SchemaConfiguration: mustSchema(t, nil, wgSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "namespaceCreate",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
					DisableDefaultMapping: false,
					Path:                  []string{},
				},
			},
			DisableResolveFieldPositions: true,
			DefaultFlushIntervalMillis:   500,
		},
	))

	t.Run("mutation with single __typename field on union", RunTest(wgSchema, `
		mutation CreateNamespace($name: String! $personal: Boolean!) {
			namespaceCreate(input: {name: $name, personal: $personal}){
				__typename
			}
		}`, "CreateNamespace",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://api.com","body":{"query":"mutation($name: String!, $personal: Boolean!){namespaceCreate(input: {name: $name,personal: $personal}){__typename}}","variables":{"personal":$$1$$,"name":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ContextVariable{
									Path:     []string{"name"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
								},
								&resolve.ContextVariable{
									Path:     []string{"personal"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("namespaceCreate"),
							Value: &resolve.Object{
								Path: []string{"namespaceCreate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("__typename"),
										Value: &resolve.String{
											Path:       []string{"__typename"},
											Nullable:   false,
											IsTypeName: true,
										},
									},
								}}},
					},
				},
			},
		}, plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName: "Mutation",
								FieldNames: []string{
									"namespaceCreate",
								},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName: "NamespaceCreated",
								FieldNames: []string{
									"namespace",
								},
							},
							{
								TypeName:   "Namespace",
								FieldNames: []string{"id", "name"},
							},
							{
								TypeName:   "Error",
								FieldNames: []string{"code", "message"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL:    "http://api.com",
							Method: "POST",
						},
						Subscription: &SubscriptionConfiguration{
							URL: "ws://api.com",
						},
						SchemaConfiguration: mustSchema(t, nil, wgSchema),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Mutation",
					FieldName: "namespaceCreate",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "input",
							SourceType: plan.FieldArgumentSource,
						},
					},
					DisableDefaultMapping: false,
					Path:                  []string{},
				},
			},
			DisableResolveFieldPositions: true,
			DefaultFlushIntervalMillis:   500,
		}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Subscription", func(t *testing.T) {
		t.Run("Subscription", runTestOnTestDefinition(t, `
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
			Response: &resolve.GraphQLSubscription{
				Trigger: resolve.GraphQLSubscriptionTrigger{
					Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`),
					Source: &SubscriptionSource{
						NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, ctx),
					},
					PostProcessing: DefaultPostProcessingConfiguration,
				},
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fields: []*resolve.Field{
							{
								Name: []byte("remainingJedis"),
								Value: &resolve.Integer{
									Path:     []string{"remainingJedis"},
									Nullable: false,
								},
							},
						},
					},
				},
			},
		}))
	})

	t.Run("Subscription with variables", RunTest(`
		type Subscription {
			foo(bar: String): Int!
 		}
`, `
		subscription SubscriptionWithVariables {
			foo(bar: "baz")
		}
	`, "SubscriptionWithVariables", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription($a: String){foo(bar: $a)}","variables":{"a":$$0$$}}}`),
				Variables: resolve.NewVariables(
					&resolve.ContextVariable{
						Path:     []string{"a"},
						Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","null"]}`),
					},
				),
				Source: &SubscriptionSource{
					client: NewGraphQLSubscriptionClient(http.DefaultClient, http.DefaultClient, ctx),
				},
				PostProcessing: DefaultPostProcessingConfiguration,
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Value: &resolve.Integer{
								Path:     []string{"foo"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Subscription",
							FieldNames: []string{"foo"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Subscription: &SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, `
						type Subscription {
							foo(bar: String): Int!
						}
					`),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Subscription",
				FieldName: "foo",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "bar",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("federation", RunTest(federationTestSchema,
		`	query MyReviews {
						me {
							id
							username
							reviews {
								body
								author {
									id
									username
								}	
								product {
									name
									price
									reviews {
										body
										author {
											id
											username
										}
									}
								}
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{me {id username __typename}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("me"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {reviews {body author {id username} product {reviews {body author {id username}} __typename upc}}}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: []resolve.Variable{
											&resolve.ResolvableObjectVariable{
												Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												}),
											},
										},
										DataSource:                            &Source{},
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										RequiresEntityFetch:                   true,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("author"),
														Value: &resolve.Object{
															Path: []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																},
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path: []string{"username"},
																	},
																},
															},
														},
													},
													{
														Name: []byte("product"),
														Value: &resolve.Object{
															Path: []string{"product"},
															Fetch: &resolve.SingleFetch{
																FetchDependencies: resolve.FetchDependencies{
																	FetchID:           2,
																	DependsOnFetchIDs: []int{1},
																},
																FetchConfiguration: resolve.FetchConfiguration{
																	Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {name price}}}","variables":{"representations":[$$0$$]}}}`,
																	DataSource: &Source{},
																	Variables: []resolve.Variable{
																		&resolve.ResolvableObjectVariable{
																			Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																				Nullable: true,
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path: []string{"__typename"},
																						},
																						OnTypeNames: [][]byte{[]byte("Product")},
																					},
																					{
																						Name: []byte("upc"),
																						Value: &resolve.String{
																							Path: []string{"upc"},
																						},
																						OnTypeNames: [][]byte{[]byte("Product")},
																					},
																				},
																			}),
																		},
																	},
																	RequiresEntityBatchFetch:              true,
																	PostProcessing:                        EntitiesPostProcessingConfiguration,
																	SetTemplateOutputToNullOnVariableNull: true,
																},
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
															},
															Fields: []*resolve.Field{
																{
																	Name: []byte("name"),
																	Value: &resolve.String{
																		Path: []string{"name"},
																	},
																},
																{
																	Name: []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																},
																{
																	Name: []byte("reviews"),
																	Value: &resolve.Array{
																		Nullable: true,
																		Path:     []string{"reviews"},
																		Item: &resolve.Object{
																			Nullable: true,
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("body"),
																					Value: &resolve.String{
																						Path: []string{"body"},
																					},
																				},
																				{
																					Name: []byte("author"),
																					Value: &resolve.Object{
																						Path: []string{"author"},
																						Fields: []*resolve.Field{
																							{
																								Name: []byte("id"),
																								Value: &resolve.String{
																									Path: []string{"id"},
																								},
																							},
																							{
																								Name: []byte("username"),
																								Value: &resolve.String{
																									Path: []string{"username"},
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
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: `extend type Query {me: User} type User @key(fields: "id"){ id: ID! username: String!}`,
							},
							`type Query {me: User} type User @key(fields: "id"){ id: ID! username: String!}`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"topProducts"},
							},
							{
								TypeName:   "Subscription",
								FieldNames: []string{"updatedPrice"},
							},
							{
								TypeName:   "Product",
								FieldNames: []string{"upc", "name", "price"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Product",
									SelectionSet: "upc",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://product.service",
						},
						Subscription: &SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: `extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: "upc") {upc: String! name: String! price: Int!}`,
							},
							`type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: "upc"){upc: String! name: String! price: Int!}`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-3",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username", "reviews"},
							},
							{
								TypeName:   "Product",
								FieldNames: []string{"upc", "reviews"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Review",
								FieldNames: []string{"body", "author", "product"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "Product",
									SelectionSet: "upc",
								},
							},
							Provides: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Review",
									FieldName:    "author",
									SelectionSet: "username",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: `type Review { body: String! author: User! @provides(fields: "username") product: Product! } extend type User @key(fields: "id") { id: ID! username: String! @external reviews: [Review] } extend type Product @key(fields: "upc") { upc: String! reviews: [Review] }`,
							},
							`type Review { body: String! author: User! @provides(fields: "username") product: Product! } type User @key(fields: "id") { id: ID! username: String! @external reviews: [Review] } type Product @key(fields: "upc") { upc: String! reviews: [Review] }`,
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "topProducts",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "first",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "User",
					FieldName: "reviews",
				},
				{
					TypeName:  "Product",
					FieldName: "name",
				},
				{
					TypeName:  "Product",
					FieldName: "price",
				},
				{
					TypeName:  "Product",
					FieldName: "reviews",
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("simple parallel federation queries", RunTest(complexFederationSchema,
		`	query Parallel {
					  user(id: "1") {
						username
					  }
					  vehicle(id: "2") {
						description
                      }
					}`,
		"Parallel",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.ParallelFetch{
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 0,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:      `{"method":"POST","url":"http://user.service","body":{"query":"query($a: ID!){user(id: $a){username}}","variables":{"a":$$0$$}}}`,
									DataSource: &Source{},
									Variables: resolve.NewVariables(
										&resolve.ContextVariable{
											Path:     []string{"a"},
											Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
										},
									),
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
							&resolve.SingleFetch{
								FetchDependencies: resolve.FetchDependencies{
									FetchID: 1,
								},
								FetchConfiguration: resolve.FetchConfiguration{
									Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($b: String!){vehicle(id: $b){description}}","variables":{"b":$$0$$}}}`,
									DataSource: &Source{},
									Variables: resolve.NewVariables(
										&resolve.ContextVariable{
											Path:     []string{"b"},
											Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string"]}`),
										},
									),
									PostProcessing: DefaultPostProcessingConfiguration,
								},
								DataSourceIdentifier: []byte("graphql_datasource.Source"),
							},
						},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path:     []string{"username"},
											Nullable: true,
										},
									},
								},
							},
						},
						{
							Name: []byte("vehicle"),
							Value: &resolve.Object{
								Path:     []string{"vehicle"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("description"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"description"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
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
								FieldNames: []string{"id", "username"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { me: User user(id: ID!): User} extend type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
							},
							"type Query { me: User user(id: ID!): User} type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"vehicle"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Vehicle",
								FieldNames: []string{"id", "name", "description", "price"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://product.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} extend type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea extend type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
							},
							"type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea extend type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "user",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
				{
					TypeName:  "Query",
					FieldName: "vehicle",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		},
		WithMultiFetchPostProcessor(),
	))

	t.Run("complex nested federation", RunTest(complexFederationSchema,
		`	query User {
					  user(id: "2") {
						id
						name {
						  first
						  last
						}
						username
						birthDate
						vehicle {
						  id
						  description
						  price
						  __typename
						}
						account {
						  ... on PasswordAccount {
							email
						  }
						  ... on SMSAccount {
							number
						  }
						}
						metadata {
						  name
						  address
						  description
						}
						ssn
					  }
					}`,
		"User",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://user.service","body":{"query":"query($a: ID!){user(id: $a){id name {first last} username birthDate account {__typename ... on PasswordAccount {email} ... on SMSAccount {number}} metadata {name address description} ssn __typename}}","variables":{"a":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ObjectVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										RequiresEntityFetch: true,
										Input:               `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {vehicle {id description price __typename}}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: []resolve.Variable{
											&resolve.ResolvableObjectVariable{
												Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												}),
											},
										},
										DataSource:                            &Source{},
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.Object{
											Path:     []string{"name"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("first"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"first"},
													},
												},
												{
													Name: []byte("last"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"last"},
													},
												},
											},
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path:     []string{"username"},
											Nullable: true,
										},
									},
									{
										Name: []byte("birthDate"),
										Value: &resolve.String{
											Path:     []string{"birthDate"},
											Nullable: true,
										},
									},
									{
										Name: []byte("vehicle"),
										Value: &resolve.Object{
											Path:     []string{"vehicle"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("description"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"description"},
													},
												},
												{
													Name: []byte("price"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"price"},
													},
												},
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path:       []string{"__typename"},
														IsTypeName: true,
													},
												},
											},
										},
									},
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("email"),
													Value: &resolve.String{
														Path: []string{"email"},
													},
													OnTypeNames: [][]byte{[]byte("PasswordAccount")},
												},
												{
													Name: []byte("number"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"number"},
													},
													OnTypeNames: [][]byte{[]byte("SMSAccount")},
												},
											},
										},
									},
									{
										Name: []byte("metadata"),
										Value: &resolve.Array{
											Path:     []string{"metadata"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"name"},
														},
													},
													{
														Name: []byte("address"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"address"},
														},
													},
													{
														Name: []byte("description"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"description"},
														},
													},
												},
											},
										},
									},
									{
										Name: []byte("ssn"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"ssn"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me", "user"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "name", "username", "birthDate", "account", "metadata", "ssn"},
							},
							{
								TypeName:   "UserMetadata",
								FieldNames: []string{"name", "address", "description"},
							},
							{
								TypeName:   "Name",
								FieldNames: []string{"first", "last"},
							},
							{
								TypeName:   "PasswordAccount",
								FieldNames: []string{"email"},
							},
							{
								TypeName:   "SMSAccount",
								FieldNames: []string{"number"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { me: User user(id: ID!): User} extend type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
							},
							"type Query { me: User user(id: ID!): User} type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"vehicle"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Vehicle",
								FieldNames: []string{"id", "name", "description", "price"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "Product",
									SelectionSet: "upc",
								},
								{
									TypeName:     "Product",
									SelectionSet: "sku",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://product.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} extend type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea extend type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
							},
							"type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "user",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("complex nested federation different order", RunTest(complexFederationSchema,
		`	query User {
					  user(id: "2") {
						id
						name {
						  first
						  last
						}
						username
						birthDate
						account {
						  ... on PasswordAccount {
							email
						  }
						  ... on SMSAccount {
							number
						  }
						}
						metadata {
						  name
						  address
						  description
						}
						vehicle {
						  id
						  description
						  price
						  __typename
						}
						ssn
					  }
					}`,
		"User",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:      `{"method":"POST","url":"http://user.service","body":{"query":"query($a: ID!){user(id: $a){id name {first last} username birthDate account {__typename ... on PasswordAccount {email} ... on SMSAccount {number}} metadata {name address description} ssn __typename}}","variables":{"a":$$0$$}}}`,
							DataSource: &Source{},
							Variables: resolve.NewVariables(
								&resolve.ObjectVariable{
									Path:     []string{"a"},
									Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
								},
							),
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {vehicle {id description price __typename}}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: []resolve.Variable{
											&resolve.ResolvableObjectVariable{
												Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												}),
											},
										},
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.Object{
											Path:     []string{"name"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("first"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"first"},
													},
												},
												{
													Name: []byte("last"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"last"},
													},
												},
											},
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path:     []string{"username"},
											Nullable: true,
										},
									},
									{
										Name: []byte("birthDate"),
										Value: &resolve.String{
											Path:     []string{"birthDate"},
											Nullable: true,
										},
									},
									{
										Name: []byte("account"),
										Value: &resolve.Object{
											Path:     []string{"account"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("email"),
													Value: &resolve.String{
														Path: []string{"email"},
													},
													OnTypeNames: [][]byte{[]byte("PasswordAccount")},
												},
												{
													Name: []byte("number"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"number"},
													},
													OnTypeNames: [][]byte{[]byte("SMSAccount")},
												},
											},
										},
									},
									{
										Name: []byte("metadata"),
										Value: &resolve.Array{
											Path:     []string{"metadata"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"name"},
														},
													},
													{
														Name: []byte("address"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"address"},
														},
													},
													{
														Name: []byte("description"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"description"},
														},
													},
												},
											},
										},
									},
									{
										Name: []byte("vehicle"),
										Value: &resolve.Object{
											Path:     []string{"vehicle"},
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path: []string{"id"},
													},
												},
												{
													Name: []byte("description"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"description"},
													},
												},
												{
													Name: []byte("price"),
													Value: &resolve.String{
														Nullable: true,
														Path:     []string{"price"},
													},
												},
												{
													Name: []byte("__typename"),
													Value: &resolve.String{
														Path:       []string{"__typename"},
														IsTypeName: true,
													},
												},
											},
										},
									},
									{
										Name: []byte("ssn"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"ssn"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me", "user"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "name", "username", "birthDate", "account", "metadata", "ssn"},
							},
							{
								TypeName:   "UserMetadata",
								FieldNames: []string{"name", "address", "description"},
							},
							{
								TypeName:   "Name",
								FieldNames: []string{"first", "last"},
							},
							{
								TypeName:   "PasswordAccount",
								FieldNames: []string{"email"},
							},
							{
								TypeName:   "SMSAccount",
								FieldNames: []string{"number"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { me: User user(id: ID!): User} extend type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
							},
							"type Query { me: User user(id: ID!): User} type Mutation { login( username: String! password: String! ): User} type User @key(fields: \"id\") { id: ID! name: Name username: String birthDate(locale: String): String account: AccountType metadata: [UserMetadata] ssn: String} type Name { first: String last: String } type PasswordAccount @key(fields: \"email\") { email: String! } type SMSAccount @key(fields: \"number\") { number: String } union AccountType = PasswordAccount | SMSAccount type UserMetadata { name: String address: String description: String }",
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"vehicle"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Vehicle",
								FieldNames: []string{"id", "name", "description", "price"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://product.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} extend type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea extend type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
							},
							"type Query { product(upc: String!): Product vehicle(id: String!): Vehicle topProducts(first: Int = 5): [Product] topCars(first: Int = 5): [Car]} type Subscription { updatedPrice: Product! updateProductPrice(upc: String!): Product! stock: [Product!]} type Ikea { asile: Int} type Amazon { referrer: String } union Brand = Ikea | Amazon interface Product { upc: String! sku: String! name: String price: String details: ProductDetails inStock: Int! } interface ProductDetails { country: String} type ProductDetailsFurniture implements ProductDetails { country: String color: String} type ProductDetailsBook implements ProductDetails { country: String pages: Int } type Furniture implements Product @key(fields: \"upc\") @key(fields: \"sku\") { upc: String! sku: String! name: String price: String brand: Brand metadata: [MetadataOrError] details: ProductDetailsFurniture inStock: Int!} interface Vehicle { id: String! description: String price: String } type Car implements Vehicle @key(fields: \"id\") { id: String! description: String price: String} type Van implements Vehicle @key(fields: \"id\") { id: String! description: String price: String } union Thing = Car | Ikea type User @key(fields: \"id\") { id: ID! @external vehicle: Vehicle thing: Thing} type KeyValue { key: String! value: String! } type Error { code: Int message: String} union MetadataOrError = KeyValue | Error",
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "user",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "id",
							SourceType: plan.FieldArgumentSource,
						},
					},
					Path: []string{"user"},
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("federation with variables", RunTest(federationTestSchema,
		`	query MyReviews($publicOnly: Boolean!, $someSkipCondition: Boolean!) {
						me {
							reviews {
								body
								notes @skip(if: $someSkipCondition)
								likes(filterToPublicOnly: $publicOnly)
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{me {__typename id}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("me"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!, $someSkipCondition: Boolean!, $publicOnly: Boolean!){_entities(representations: $representations){__typename ... on User {reviews {body notes @skip(if: $someSkipCondition) likes(filterToPublicOnly: $publicOnly)}}}}","variables":{"representations":[$$2$$],"publicOnly":$$1$$,"someSkipCondition":$$0$$}}}`,
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"someSkipCondition"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"publicOnly"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
											},
											&resolve.ResolvableObjectVariable{
												Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												}),
											},
										),
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("notes"),
														Value: &resolve.String{
															Path:     []string{"notes"},
															Nullable: true,
														},
														SkipDirectiveDefined: true,
														SkipVariableName:     "someSkipCondition",
													},
													{
														Name: []byte("likes"),
														Value: &resolve.String{
															Path: []string{"likes"},
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! }",
							},
							"type Query {me: User} type User @key(fields: \"id\"){ id: ID! }",
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"reviews", "id"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Review",
								FieldNames: []string{"body", "notes", "likes"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "type Review { body: String! notes: String likes(filterToPublicOnly: Boolean): Int! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] }",
							},
							"type Review { body: String! notes: String likes(filterToPublicOnly: Boolean): Int! } type User @key(fields: \"id\") { id: ID! @external reviews: [Review] }",
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Review",
					FieldName: "likes",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "filterToPublicOnly",
							SourceType: plan.FieldArgumentSource,
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("federation with variables and renamed types", RunTest(federationTestSchema,
		`	query MyReviews($publicOnly: Boolean!, $someSkipCondition: Boolean!) {
						me {
							reviews {
								body
								notes @skip(if: $someSkipCondition)
								likes(filterToPublicOnly: $publicOnly)
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{me {__typename id}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("me"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!, $someSkipCondition: Boolean!, $publicOnly: XBoolean!){_entities(representations: $representations){__typename ... on User {reviews {body notes @skip(if: $someSkipCondition) likes(filterToPublicOnly: $publicOnly)}}}}","variables":{"representations":[$$2$$],"publicOnly":$$1$$,"someSkipCondition":$$0$$}}}`,
										Variables: resolve.NewVariables(
											&resolve.ContextVariable{
												Path:     []string{"someSkipCondition"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean"]}`),
											},
											&resolve.ContextVariable{
												Path:     []string{"publicOnly"},
												Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["boolean","null"]}`),
											},
											resolve.NewResolvableObjectVariable(
												&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.Scalar{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												},
											),
										),
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("notes"),
														Value: &resolve.String{
															Path:     []string{"notes"},
															Nullable: true,
														},
														SkipDirectiveDefined: true,
														SkipVariableName:     "someSkipCondition",
													},
													{
														Name: []byte("likes"),
														Value: &resolve.String{
															Path: []string{"likes"},
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! }",
							},
							federationTestSchemaWithRename,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "reviews"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Review",
								FieldNames: []string{"body", "notes", "likes"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "scalar XBoolean type Review { body: String! notes: String likes(filterToPublicOnly: XBoolean!): Int! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] }",
							},
							federationTestSchemaWithRename,
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Review",
					FieldName: "likes",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:         "filterToPublicOnly",
							SourceType:   plan.FieldArgumentSource,
							RenameTypeTo: "XBoolean",
						},
					},
				},
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("federated entity with requires", RunTest(requiredFieldTestSchema,
		`	query QueryWithRequiredFields {
						serviceOne {
							serviceTwoFieldOne  # @requires(fields: "serviceOneFieldOne")
							serviceTwoFieldTwo  # @requires(fields: "serviceOneFieldTwo")
						}
					}`,
		"QueryWithRequiredFields",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							// Should fetch the federation key as well as all the required fields.
							Input:          `{"method":"POST","url":"http://one.service","body":{"query":"{serviceOne {serviceOneFieldOne serviceOneFieldTwo __typename id}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("serviceOne"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										// The required fields are present in the representations.
										Input: `{"method":"POST","url":"http://two.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on ServiceOneType {serviceTwoFieldOne serviceTwoFieldTwo}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: resolve.NewVariables(
											resolve.NewResolvableObjectVariable(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("__typename"),
														Value: &resolve.String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("ServiceOneType")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("ServiceOneType")},
													},
													{
														Name: []byte("serviceOneFieldOne"),
														Value: &resolve.String{
															Path: []string{"serviceOneFieldOne"},
														},
														OnTypeNames: [][]byte{[]byte("ServiceOneType")},
													},
													{
														Name: []byte("serviceOneFieldTwo"),
														Value: &resolve.String{
															Path: []string{"serviceOneFieldTwo"},
														},
														OnTypeNames: [][]byte{[]byte("ServiceOneType")},
													},
												},
											}),
										),
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"serviceOne"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("serviceTwoFieldOne"),
										Value: &resolve.String{
											Path: []string{"serviceTwoFieldOne"},
										},
									},
									{
										Name: []byte("serviceTwoFieldTwo"),
										Value: &resolve.String{
											Path: []string{"serviceTwoFieldTwo"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"serviceOne"},
							},
							{
								TypeName:   "ServiceOneType",
								FieldNames: []string{"id", "serviceOneFieldOne", "serviceOneFieldTwo"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "ServiceOneType",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://one.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query {serviceOne: ServiceOneType} type ServiceOneType @key(fields: \"id\"){ id: ID! serviceOneFieldOne: String! serviceOneFieldTwo: String!}",
							},
							"type Query {serviceOne: ServiceOneType} type ServiceOneType @key(fields: \"id\"){ id: ID! serviceOneFieldOne: String! serviceOneFieldTwo: String!}",
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "ServiceOneType",
								FieldNames: []string{"id", "serviceTwoFieldOne", "serviceTwoFieldTwo"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "ServiceOneType",
									SelectionSet: "id",
								},
							},
							Requires: []plan.FederationFieldConfiguration{
								{
									TypeName:     "ServiceOneType",
									FieldName:    "serviceTwoFieldOne",
									SelectionSet: "serviceOneFieldOne",
								},
								{
									TypeName:     "ServiceOneType",
									FieldName:    "serviceTwoFieldTwo",
									SelectionSet: "serviceOneFieldTwo",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://two.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type ServiceOneType @key(fields: \"id\") { id: ID! @external serviceOneFieldOne: String! @external serviceOneFieldTwo: String! @external serviceTwoFieldOne: String! @requires(fields: \"serviceOneFieldOne\") serviceTwoFieldTwo: String! @requires(fields: \"serviceOneFieldTwo\")}",
							},
							"type ServiceOneType @key(fields: \"id\") { id: ID! @external serviceOneFieldOne: String! @external serviceOneFieldTwo: String! @external serviceTwoFieldOne: String! @requires(fields: \"serviceOneFieldOne\") serviceTwoFieldTwo: String! @requires(fields: \"serviceOneFieldTwo\")}",
						),
					}),
				),
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("federation with renamed schema", RunTest(renamedFederationTestSchema,
		`	query MyReviews {
						api_me {
							id
							username
							reviews {
								body
								author {
									id
									username
								}	
								product {
									name
									price
									reviews {
										body
										author {
											id
											username
										}
									}
								}
							}
						}
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{api_me: me {id username}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("api_me"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {reviews {body author {id username} product {reviews {body author {id username}} upc}}}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: resolve.NewVariables(
											resolve.NewResolvableObjectVariable(&resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("__typename"),
														Value: &resolve.String{
															Path: []string{"__typename"},
														},
														OnTypeNames: [][]byte{[]byte("User")},
													},
													{
														Name: []byte("id"),
														Value: &resolve.Scalar{
															Path: []string{"id"},
														},
														OnTypeNames: [][]byte{[]byte("User")},
													},
												},
											}),
										),
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"api_me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("reviews"),
										Value: &resolve.Array{
											Path:     []string{"reviews"},
											Nullable: true,
											Item: &resolve.Object{
												Nullable: true,
												Fields: []*resolve.Field{
													{
														Name: []byte("body"),
														Value: &resolve.String{
															Path: []string{"body"},
														},
													},
													{
														Name: []byte("author"),
														Value: &resolve.Object{
															Path: []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																},
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path: []string{"username"},
																	},
																},
															},
														},
													},
													{
														Name: []byte("product"),
														Value: &resolve.Object{
															Path: []string{"product"},
															Fetch: &resolve.SingleFetch{
																FetchConfiguration: resolve.FetchConfiguration{
																	Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Product {name price}}}","variables":{"representations":[{"upc":$$0$$,"__typename":"Product"}]}}}`,
																	DataSource: &Source{},
																	Variables: resolve.NewVariables(
																		resolve.NewResolvableObjectVariable(
																			&resolve.Object{
																				Nullable: true,
																				Fields: []*resolve.Field{
																					{
																						Name: []byte("__typename"),
																						Value: &resolve.String{
																							Path: []string{"__typename"},
																						},
																						OnTypeNames: [][]byte{[]byte("Product")},
																					},
																					{
																						Name: []byte("upc"),
																						Value: &resolve.String{
																							Path: []string{"upc"},
																						},
																						OnTypeNames: [][]byte{[]byte("Product")},
																					},
																				},
																			},
																		),
																	),
																	RequiresEntityBatchFetch:              true,
																	PostProcessing:                        EntitiesPostProcessingConfiguration,
																	SetTemplateOutputToNullOnVariableNull: true,
																},
																DataSourceIdentifier: []byte("graphql_datasource.Source"),
															},
															Fields: []*resolve.Field{
																{
																	Name: []byte("name"),
																	Value: &resolve.String{
																		Path: []string{"name"},
																	},
																},
																{
																	Name: []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																},
																{
																	Name: []byte("reviews"),
																	Value: &resolve.Array{
																		Nullable: true,
																		Path:     []string{"reviews"},
																		Item: &resolve.Object{
																			Nullable: true,
																			Fields: []*resolve.Field{
																				{
																					Name: []byte("body"),
																					Value: &resolve.String{
																						Path: []string{"body"},
																					},
																				},
																				{
																					Name: []byte("author"),
																					Value: &resolve.Object{
																						Path: []string{"author"},
																						Fields: []*resolve.Field{
																							{
																								Name: []byte("id"),
																								Value: &resolve.String{
																									Path: []string{"id"},
																								},
																							},
																							{
																								Name: []byte("username"),
																								Value: &resolve.String{
																									Path: []string{"username"},
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
		},
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"api_me"},
							},
							{
								TypeName:   "User_api",
								FieldNames: []string{"id", "username"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! username: String!}",
							},
							federationTestSchema,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"api_topProducts"},
							},
							{
								TypeName:   "Product_api",
								FieldNames: []string{"upc", "name", "price"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Product",
									SelectionSet: "upc",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://product.service",
						},
						Subscription: &SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") {upc: String! price: Int!}",
							},
							federationTestSchema,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User_api",
								FieldNames: []string{"id", "username", "reviews"},
							},
							{
								TypeName:   "Product_api",
								FieldNames: []string{"upc", "reviews"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Review_api",
								FieldNames: []string{"body", "author", "product"},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") { upc: String! @external reviews: [Review] }",
							},
							federationTestSchema,
						),
					}),
				),
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:  "Query",
					FieldName: "topProducts_api",
					Arguments: []plan.ArgumentConfiguration{
						{
							Name:       "first",
							SourceType: plan.FieldArgumentSource,
						},
					},
					Path: []string{"topProducts"},
				},
				{
					TypeName:  "Query",
					FieldName: "api_me",
					Path:      []string{"me"},
				},
			},
			DisableResolveFieldPositions: true,
			Types: []plan.TypeConfiguration{
				{
					TypeName: "User_api",
					RenameTo: "User",
				},
				{
					TypeName: "Product_api",
					RenameTo: "Product",
				},
				{
					TypeName: "Review_api",
					RenameTo: "Review",
				},
			},
		},
		WithSkipReason("Renaming is broken. it is unclear how metadata should look like with renamed types"),
	))

	t.Run("userSDLWithInterface + reviewSDL", func(t *testing.T) {

		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"me", "self"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Identity",
								FieldNames: []string{"id"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: userSDLWithInterface,
							},
							`
							type Query {
								me: User
								self: Identity
							}
							
							interface Identity {
								id: ID!
							}
							
							type User implements Identity @key(fields: "id") {
								id: ID!
								username: String!
							}
						`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "reviews"},
							},
							{
								TypeName:   "Review",
								FieldNames: []string{"id", "body", "author", "attachment"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Medium",
								FieldNames: []string{"size"},
							},
							{
								TypeName:   "Image",
								FieldNames: []string{"size", "extension"},
							},
							{
								TypeName:   "Video",
								FieldNames: []string{"size", "length"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
								{
									TypeName:     "Review",
									SelectionSet: "id",
								},
							},
							Provides: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Review",
									FieldName:    "author",
									SelectionSet: "username",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://review.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: reviewSDL,
							},
							`
							interface Medium {
								size: Int!
							}
						
							type Image implements Medium {
								size: Int!
								extension: String!
							}
						
							type Video implements Medium {
								size: Int!
								length: Int!
							}
						
							type Review @key(fields: "id") {
								id: ID!
								body: String!
								author: User! @provides(fields: "username")
								attachment: Medium
							}
							
							type User @key(fields: "id") {
								id: ID!
								reviews: [Review] 
							}`,
						),
					}),
				),
			},
			DisableResolveFieldPositions: true,
		}

		t.Run("federation with object query and inline fragment", RunTest(federatedSchemaWithInterfaceQuery,
			`
				query ObjectQuery {
				  me {
					id
					__typename
					... on User {
					  uid: id
					  username
					  reviews {
						body
					  }
					}
				  }
				}`,
			"ObjectQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{me {id __typename uid: id username}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("me"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {reviews {body}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"me"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.String{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												Nullable:   false,
												IsTypeName: true,
											},
										},
										{
											Name: []byte("uid"),
											Value: &resolve.String{
												Path: []string{"uid"},
											},
										},
										{
											Name: []byte("username"),
											Value: &resolve.String{
												Path: []string{"username"},
											},
										},
										{
											Name: []byte("reviews"),
											Value: &resolve.Array{
												Path:     []string{"reviews"},
												Nullable: true,
												Item: &resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("body"),
															Value: &resolve.String{
																Path: []string{"body"},
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
			planConfiguration,
		))

		t.Run("federation with interface query", RunTest(federatedSchemaWithInterfaceQuery,
			`
			query InterfaceQuery {
			  self {
				id
				__typename
				... on User {
				  uid: id
				  username
				  reviews {
					body
				  }
				}
			  }
			}`,
			"InterfaceQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{self {id __typename ... on User {uid: id username __typename id}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("self"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {reviews {body}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"self"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("id"),
											Value: &resolve.String{
												Path: []string{"id"},
											},
										},
										{
											Name: []byte("__typename"),
											Value: &resolve.String{
												Path:       []string{"__typename"},
												Nullable:   false,
												IsTypeName: true,
											},
										},
										{
											Name: []byte("uid"),
											Value: &resolve.String{
												Path: []string{"uid"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("username"),
											Value: &resolve.String{
												Path: []string{"username"},
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
										{
											Name: []byte("reviews"),
											Value: &resolve.Array{
												Path:     []string{"reviews"},
												Nullable: true,
												Item: &resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("body"),
															Value: &resolve.String{
																Path: []string{"body"},
															},
														},
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration,
		))

		t.Run("Federation with query returning interface that features nested interfaces", RunTest(federatedSchemaWithInterfaceQuery,
			`
		query InterfaceQuery {
		  self {
			... on User {
			  reviews {
				body
				attachment {
					... on Image {
						extension
					}
					... on Video {
						length
					}
				}
			  }
			}
		  }
		}`,
			"InterfaceQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{self {__typename ... on User {__typename id}}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("self"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {reviews {body attachment {__typename ... on Image {extension} ... on Video {length}}}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"self"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("reviews"),
											Value: &resolve.Array{
												Path:     []string{"reviews"},
												Nullable: true,
												Item: &resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("body"),
															Value: &resolve.String{
																Path: []string{"body"},
															},
														},
														{
															Name: []byte("attachment"),
															Value: &resolve.Object{
																Path: []string{"attachment"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("extension"),
																		Value: &resolve.String{
																			Path: []string{"extension"},
																		},
																		OnTypeNames: [][]byte{[]byte("Image")},
																	},
																	{
																		Name: []byte("length"),
																		Value: &resolve.String{
																			Path: []string{"length"},
																		},
																		OnTypeNames: [][]byte{[]byte("Video")},
																	},
																},
															},
														},
													},
												},
											},
											OnTypeNames: [][]byte{[]byte("User")},
										},
									},
								},
							},
						},
					},
				},
			},
			planConfiguration,
		))

	})

	// When user is an entity, the "pets" field can be both declared and resolved only in the pet subgraph
	// This separation of concerns is recommended: https://www.apollographql.com/docs/federation/v1/#separation-of-concerns
	t.Run("Federation with interface field query (defined on pet subgraph)", func(t *testing.T) {
		planConfiguration := plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id-1",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"user"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: simpleUserSchema,
							},
							`
							type Query {
								user: User
							}
							type User @key(fields: "id") {
								id: ID!
								username: String!
							}
						`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "pets"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Details",
								FieldNames: []string{"age", "hasOwner"},
							},
							{
								TypeName:   "Pet",
								FieldNames: []string{"name", "species", "details"},
							},
							{
								TypeName:   "Cat",
								FieldNames: []string{"catField", "name", "species", "details"},
							},
							{
								TypeName:   "Dog",
								FieldNames: []string{"dogField", "name", "species", "details"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://pet.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: petSchema,
							},
							`
							type Details {
								age: Int!
								hasOwner : Boolean!
							}
							interface Pet {
								name: String!
								species: String!
								details: Details!
							}
							type Cat implements Pet {
								name: String!
								species: String!
								catField: String!
								details: Details!
							}
							type Dog implements Pet {
								name: String!
								species: String!
								dogField: String!
								details: Details!
							}
							type User @key(fields: "id") {
								id: ID! @external
								pets: [Pet!]!
							}
						`,
						),
					}),
				),
			},
			DisableResolveFieldPositions: true,
			Debug: plan.DebugConfiguration{
				PrintOperationTransformations: false,
			},
		}

		t.Run("featuring consecutive inline fragments (shared selection at top)", RunTest(
			federatedSchemaWithComplexNestedFragments,
			`
		query TestQuery {
			user {
				username
				pets {
					name
					... on Cat {
						catField
						details {
							age
						}
					}
					... on Dog {
						dogField
						species
					}
					details {
						hasOwner
					}
				}
			}
		}
		`,
			"TestQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {username __typename id}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://pet.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {pets {name __typename ... on Cat {catField details {age}} ... on Dog {dogField species} details {hasOwner}}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("username"),
											Value: &resolve.String{
												Path: []string{"username"},
											},
										},
										{
											Name: []byte("pets"),
											Value: &resolve.Array{
												Path:     []string{"pets"},
												Nullable: false,
												Item: &resolve.Object{
													Nullable: false,
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Value: &resolve.String{
																Path: []string{"name"},
															},
														},
														{
															Name: []byte("catField"),
															Value: &resolve.String{
																Path: []string{"catField"},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("age"),
																		Value: &resolve.Integer{
																			Path: []string{"age"},
																		},
																	},
																},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("dogField"),
															Value: &resolve.String{
																Path: []string{"dogField"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("species"),
															Value: &resolve.String{
																Path: []string{"species"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("hasOwner"),
																		Value: &resolve.Boolean{
																			Path: []string{"hasOwner"},
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
			planConfiguration))

		t.Run("featuring consecutive inline fragments (shared selection in middle)", RunTest(
			federatedSchemaWithComplexNestedFragments,
			`
			query TestQuery {
				user {
					username
					pets {
						... on Cat {
							catField
							details {
								age
							}
						}
						name
						... on Dog {
							dogField
							species
						}
						details {
							hasOwner
						}
					}
				}
			}
			`,
			"TestQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {username __typename id}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										FetchConfiguration: resolve.FetchConfiguration{
											Input: `{"method":"POST","url":"http://pet.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {pets {__typename ... on Cat {catField details {age}} name ... on Dog {dogField species} details {hasOwner}}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											RequiresEntityFetch:                   true,
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("username"),
											Value: &resolve.String{
												Path: []string{"username"},
											},
										},
										{
											Name: []byte("pets"),
											Value: &resolve.Array{
												Path:     []string{"pets"},
												Nullable: false,
												Item: &resolve.Object{
													Nullable: false,
													Fields: []*resolve.Field{
														{
															Name: []byte("catField"),
															Value: &resolve.String{
																Path: []string{"catField"},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("age"),
																		Value: &resolve.Integer{
																			Path: []string{"age"},
																		},
																	},
																},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("name"),
															Value: &resolve.String{
																Path: []string{"name"},
															},
														},
														{
															Name: []byte("dogField"),
															Value: &resolve.String{
																Path: []string{"dogField"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("species"),
															Value: &resolve.String{
																Path: []string{"species"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("hasOwner"),
																		Value: &resolve.Boolean{
																			Path: []string{"hasOwner"},
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
			planConfiguration))

		t.Run("featuring consecutive inline fragments (shared selection at bottom)", RunTest(
			federatedSchemaWithComplexNestedFragments,
			`
			query TestQuery {
				user {
					username
					pets {
						... on Cat {
							catField
							details {
								age
							}
						}
						... on Dog {
							dogField
							species
						}
						details {
							hasOwner
						}
						name
					}
				}
			}
			`,
			"TestQuery",
			&plan.SynchronousResponsePlan{
				Response: &resolve.GraphQLResponse{
					Data: &resolve.Object{
						Fetch: &resolve.SingleFetch{
							FetchConfiguration: resolve.FetchConfiguration{
								Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {username __typename id}}"}}`,
								DataSource:     &Source{},
								PostProcessing: DefaultPostProcessingConfiguration,
							},
							DataSourceIdentifier: []byte("graphql_datasource.Source"),
						},
						Fields: []*resolve.Field{
							{
								Name: []byte("user"),
								Value: &resolve.Object{
									Fetch: &resolve.SingleFetch{
										FetchConfiguration: resolve.FetchConfiguration{
											// Note: __typename is included in the Cat and Dog inline fragments
											// because the field were originally themselves in inline fragments
											// that were inlined. The additional __typename selections are
											// harmless.
											Input: `{"method":"POST","url":"http://pet.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {pets {__typename ... on Cat {catField details {age}} ... on Dog {dogField species} details {hasOwner} name}}}}","variables":{"representations":[$$0$$]}}}`,
											Variables: resolve.NewVariables(
												&resolve.ResolvableObjectVariable{
													Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
														Nullable: true,
														Fields: []*resolve.Field{
															{
																Name: []byte("__typename"),
																Value: &resolve.String{
																	Path: []string{"__typename"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
															{
																Name: []byte("id"),
																Value: &resolve.String{
																	Path: []string{"id"},
																},
																OnTypeNames: [][]byte{[]byte("User")},
															},
														},
													}),
												},
											),
											DataSource:                            &Source{},
											PostProcessing:                        SingleEntityPostProcessingConfiguration,
											SetTemplateOutputToNullOnVariableNull: true,
											RequiresEntityFetch:                   true,
										},
										FetchDependencies: resolve.FetchDependencies{
											FetchID:           1,
											DependsOnFetchIDs: []int{0},
										},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
									},
									Path:     []string{"user"},
									Nullable: true,
									Fields: []*resolve.Field{
										{
											Name: []byte("username"),
											Value: &resolve.String{
												Path: []string{"username"},
											},
										},
										{
											Name: []byte("pets"),
											Value: &resolve.Array{
												Path:     []string{"pets"},
												Nullable: false,
												Item: &resolve.Object{
													Nullable: false,
													Fields: []*resolve.Field{
														{
															Name: []byte("catField"),
															Value: &resolve.String{
																Path: []string{"catField"},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("age"),
																		Value: &resolve.Integer{
																			Path: []string{"age"},
																		},
																	},
																},
															},
															OnTypeNames: [][]byte{[]byte("Cat")},
														},
														{
															Name: []byte("dogField"),
															Value: &resolve.String{
																Path: []string{"dogField"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("species"),
															Value: &resolve.String{
																Path: []string{"species"},
															},
															OnTypeNames: [][]byte{[]byte("Dog")},
														},
														{
															Name: []byte("details"),
															Value: &resolve.Object{
																Path: []string{"details"},
																Fields: []*resolve.Field{
																	{
																		Name: []byte("hasOwner"),
																		Value: &resolve.Boolean{
																			Path: []string{"hasOwner"},
																		},
																	},
																},
															},
														},
														{
															Name: []byte("name"),
															Value: &resolve.String{
																Path: []string{"name"},
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
			planConfiguration))
	})

	t.Run("Federation with field query (defined in pet subgraph) featuring consecutive inline union fragments", RunTest(
		`
	   type Query {
	       user: User
	   }
	   type User {
			id: ID!
	       username: String!
	       pets: [CatOrDog!]!
	   }
	   type Cat {
	       name: String!
	       catField: String!
	   }
	   type Dog {
	       name: String!
	       dogField: String!
	   }
	   union CatOrDog = Cat | Dog
	   `,
		`
	   query TestQuery {
	       user {
	           username
	           pets {
	               ... on Cat {
						name
	                   catField
	               }
	               ... on Dog {
						name
	                   dogField
	               }
	           }
	       }
	   }
	   `,
		"TestQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {username __typename id}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Fetch: &resolve.SingleFetch{
									FetchDependencies: resolve.FetchDependencies{
										FetchID:           1,
										DependsOnFetchIDs: []int{0},
									},
									FetchConfiguration: resolve.FetchConfiguration{
										Input: `{"method":"POST","url":"http://pet.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {pets {__typename ... on Cat {name catField} ... on Dog {name dogField}}}}}","variables":{"representations":[$$0$$]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ResolvableObjectVariable{
												Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("__typename"),
															Value: &resolve.String{
																Path: []string{"__typename"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
														{
															Name: []byte("id"),
															Value: &resolve.String{
																Path: []string{"id"},
															},
															OnTypeNames: [][]byte{[]byte("User")},
														},
													},
												}),
											},
										),
										DataSource:                            &Source{},
										RequiresEntityFetch:                   true,
										PostProcessing:                        SingleEntityPostProcessingConfiguration,
										SetTemplateOutputToNullOnVariableNull: true,
									},
									DataSourceIdentifier: []byte("graphql_datasource.Source"),
								},
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("pets"),
										Value: &resolve.Array{
											Path:     []string{"pets"},
											Nullable: false,
											Item: &resolve.Object{
												Nullable: false,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("Cat")},
													},
													{
														Name: []byte("catField"),
														Value: &resolve.String{
															Path: []string{"catField"},
														},
														OnTypeNames: [][]byte{[]byte("Cat")},
													},
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("Dog")},
													},
													{
														Name: []byte("dogField"),
														Value: &resolve.String{
															Path: []string{"dogField"},
														},
														OnTypeNames: [][]byte{[]byte("Dog")},
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"user"},
							},
							{
								TypeName:   "User",
								FieldNames: []string{"id", "username"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled:    true,
								ServiceSDL: simpleUserSchema,
							},
							`
							type Query {
								user: User
							}
							type User @key(fields: "id") {
								id: ID!
								username: String!
							}`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"id", "pets"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Cat",
								FieldNames: []string{"name", "catField"},
							},
							{
								TypeName:   "Dog",
								FieldNames: []string{"name", "dogField"},
							},
						}, FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "User",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://pet.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled: true,
								ServiceSDL: `
								union CatOrDog = Cat | Dog
								type Cat {
									name: String!
									catField: String!
								}
								type Dog {
									name: String!
									dogField: String!
								}
								extend type User @key(fields: "id") {
									id: ID! @external
									pets: [CatOrDog!]!
								}
	                       `,
							},
							`
								union CatOrDog = Cat | Dog
								type Cat {
									name: String!
									catField: String!
								}
								type Dog {
									name: String!
									dogField: String!
								}
								type User @key(fields: "id") {
									id: ID! @external
									pets: [CatOrDog!]!
								}
	                       `,
						),
					}),
				),
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("Federation with field query (defined in user subgraph) featuring consecutive inline union fragments", RunTest(
		`
        type Query {
            user: User
        }
        type User {
            username: String!
            pets: [CatOrDog!]!
        }
        type Cat {
			id: ID!
            name: String!
            catField: String!
        }
        type Dog {
			id: ID!
            name: String!
            dogField: String!
        }
        union CatOrDog = Cat | Dog
        `,
		`
        query TestQuery {
            user {
                username
                pets {
                    ... on Cat {
                    	name
                        catField
                    }
                    ... on Dog {
                    	name
                        dogField
                    }
                }
            }
        }
        `,
		"TestQuery",
		&plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						FetchConfiguration: resolve.FetchConfiguration{
							Input:          `{"method":"POST","url":"http://user.service","body":{"query":"{user {username pets {__typename ... on Cat {__typename id} ... on Dog {__typename id}}}}"}}`,
							DataSource:     &Source{},
							PostProcessing: DefaultPostProcessingConfiguration,
						},
						DataSourceIdentifier: []byte("graphql_datasource.Source"),
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("user"),
							Value: &resolve.Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
									},
									{
										Name: []byte("pets"),
										Value: &resolve.Array{
											Path:     []string{"pets"},
											Nullable: false,
											Item: &resolve.Object{
												Fetch: &resolve.SingleFetch{
													FetchDependencies: resolve.FetchDependencies{
														FetchID:           1,
														DependsOnFetchIDs: []int{0},
													},
													FetchConfiguration: resolve.FetchConfiguration{
														Input: `{"method":"POST","url":"http://pet.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on Cat {name catField} ... on Dog {name dogField}}}","variables":{"representations":[$$0$$]}}}`,
														Variables: []resolve.Variable{
															&resolve.ResolvableObjectVariable{
																Renderer: resolve.NewGraphQLVariableResolveRenderer(&resolve.Object{
																	Nullable: true,
																	Fields: []*resolve.Field{
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Cat")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Cat")},
																		},
																		{
																			Name: []byte("__typename"),
																			Value: &resolve.String{
																				Path: []string{"__typename"},
																			},
																			OnTypeNames: [][]byte{[]byte("Dog")},
																		},
																		{
																			Name: []byte("id"),
																			Value: &resolve.String{
																				Path: []string{"id"},
																			},
																			OnTypeNames: [][]byte{[]byte("Dog")},
																		},
																	},
																}),
															},
														},
														DataSource:                            &Source{},
														RequiresEntityBatchFetch:              true,
														PostProcessing:                        EntitiesPostProcessingConfiguration,
														SetTemplateOutputToNullOnVariableNull: true,
													},
													DataSourceIdentifier: []byte("graphql_datasource.Source"),
												},
												Nullable: false,
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("Cat")},
													},
													{
														Name: []byte("catField"),
														Value: &resolve.String{
															Path: []string{"catField"},
														},
														OnTypeNames: [][]byte{[]byte("Cat")},
													},
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														OnTypeNames: [][]byte{[]byte("Dog")},
													},

													{
														Name: []byte("dogField"),
														Value: &resolve.String{
															Path: []string{"dogField"},
														},
														OnTypeNames: [][]byte{[]byte("Dog")},
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
		plan.Configuration{
			DataSources: []plan.DataSource{
				mustDataSourceConfiguration(
					t,
					"ds-id",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"user"},
							},
							{
								TypeName:   "Cat",
								FieldNames: []string{"id"},
							},
							{
								TypeName:   "Dog",
								FieldNames: []string{"id"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "User",
								FieldNames: []string{"username", "pets"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Cat",
									SelectionSet: "id",
								},
								{
									TypeName:     "Dog",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://user.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled: true,
								ServiceSDL: `
                                extend type Query {
                                    user: User
                                }
                                type User {
                                    username: String!
									pets: [CatOrDog!]!
                                }
                                extend union CatOrDog = Cat | Dog
                                extend type Cat @key(fields: "id") {
                                    id: ID! @external
                                }
                                extend type Dog @key(fields: "id") {
                                    id: ID! @external
                                }
                            `,
							},
							`
                                type Query {
                                    user: User
                                }
                                type User {
                                    username: String!
									pets: [CatOrDog!]!
                                }
                                union CatOrDog = Cat | Dog
                                type Cat @key(fields: "id") {
                                    id: ID!
                                }
                                type Dog @key(fields: "id") {
                                    id: ID!
                                }`,
						),
					}),
				),
				mustDataSourceConfiguration(
					t,
					"ds-id-2",
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Cat",
								FieldNames: []string{"id", "name", "catField"},
							},
							{
								TypeName:   "Dog",
								FieldNames: []string{"id", "name", "dogField"},
							},
						},
						FederationMetaData: plan.FederationMetaData{
							Keys: []plan.FederationFieldConfiguration{
								{
									TypeName:     "Cat",
									SelectionSet: "id",
								},
								{
									TypeName:     "Dog",
									SelectionSet: "id",
								},
							},
						},
					},
					mustCustomConfiguration(t, ConfigurationInput{
						Fetch: &FetchConfiguration{
							URL: "http://pet.service",
						},
						SchemaConfiguration: mustSchema(t,
							&FederationConfiguration{
								Enabled: true,
								ServiceSDL: `
                                union CatOrDog = Cat | Dog
                                type Cat @key(fields: "id") {
                                    id: ID!
                                    name: String!
                                    catField: String!
                                }
                                type Dog @key(fields: "id") {
                                    id: ID!
                                    name: String!
                                    dogField: String!
                                }
                            `,
							},
							`
                                union CatOrDog = Cat | Dog
                                type Cat @key(fields: "id") {
                                    id: ID!
                                    name: String!
                                    catField: String!
                                }
                                type Dog @key(fields: "id") {
                                    id: ID!
                                    name: String!
                                    dogField: String!
                                }
                            `,
						),
					}),
				),
			},
			DisableResolveFieldPositions: true,
		}))

	t.Run("custom scalar replacement query", RunTest(starWarsSchema, `
		query MyQuery($droidId: ID!, $reviewId: ID!){
			droid(id: $droidId){
				name
				aliased: name
				friends {
					name
				}
				primaryFunction
			}
			review(id: $reviewId){
				stars
			}
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$2$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($droidId: ID!, $reviewId: ReviewID!){droid(id: $droidId){name aliased: name friends {name} primaryFunction} review(id: $reviewId){stars}}","variables":{"reviewId":$$1$$,"droidId":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"droidId"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.ContextVariable{
								Path:     []string{"reviewId"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
							&resolve.HeaderVariable{
								Path: []string{"Authorization"},
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("droid"),
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
								},
								{
									Name: []byte("friends"),
									Value: &resolve.Array{
										Nullable: true,
										Path:     []string{"friends"},
										Item: &resolve.Object{
											Nullable: true,
											Fields: []*resolve.Field{
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path: []string{"name"},
													},
												},
											},
										},
									},
								},
								{
									Name: []byte("primaryFunction"),
									Value: &resolve.String{
										Path: []string{"primaryFunction"},
									},
								},
							},
						},
					},
					{
						Name: []byte("review"),
						Value: &resolve.Object{
							Path:     []string{"review"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("stars"),
									Value: &resolve.Integer{
										Path: []string{"stars"},
									},
								},
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"droid", "review", "hero", "stringList", "nestedStringList"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Character",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
						{
							TypeName:   "Review",
							FieldNames: []string{"id", "stars", "commentary"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
					SchemaConfiguration: mustSchema(t, nil, starWarsSchemaWithRenamedArgument),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "review",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "id",
						SourceType:   plan.FieldArgumentSource,
						RenameTypeTo: "ReviewID",
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))

	t.Run("custom scalar type fields", RunTest(customUserSchema, `
		query Custom($id: ID!) {
          custom_user(id: $id) {
			id
			name
			tier
			meta {
              foo
			}
          }
		}
	`, "Custom", &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					FetchConfiguration: resolve.FetchConfiguration{
						DataSource: &Source{},
						Input:      `{"method":"POST","url":"http://localhost:8084/query","body":{"query":"query($id: ID!){custom_user: user(id: $id){id name tier meta}}","variables":{"id":$$0$$}}}`,
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:     []string{"id"},
								Renderer: resolve.NewJSONVariableRendererWithValidation(`{"type":["string","integer"]}`),
							},
						),
						PostProcessing: DefaultPostProcessingConfiguration,
					},
					DataSourceIdentifier: []byte("graphql_datasource.Source"),
				},
				Fields: []*resolve.Field{
					{
						Name: []byte("custom_user"),
						Value: &resolve.Object{
							Path:     []string{"custom_user"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("id"),
									Value: &resolve.String{
										Path: []string{"id"},
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
								},
								{
									Name: []byte("tier"),
									Value: &resolve.String{
										Nullable: true,
										Path:     []string{"tier"},
									},
								},
								{
									Name: []byte("meta"),
									Value: &resolve.Object{
										Path: []string{"meta"},
										Fields: []*resolve.Field{
											{
												Name: []byte("foo"),
												Value: &resolve.String{
													Path: []string{"foo"},
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
	}, plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfiguration(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"custom_user"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "custom_User",
							FieldNames: []string{"id", "name", "tier", "meta"},
						},
						{
							TypeName:   "custom_Meta",
							FieldNames: []string{"foo"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL: "http://localhost:8084/query",
					},
					CustomScalarTypeFields: []SingleTypeField{
						{
							TypeName:  "custom_User",
							FieldName: "meta",
						},
					},
					SchemaConfiguration: mustSchema(t, nil, userSchema),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "custom_user",
				Path:      []string{"user"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}))
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f *FailingSubscriptionClient) Subscribe(ctx *resolve.Context, options GraphQLSubscriptionOptions, updater resolve.SubscriptionUpdater) error {
	return errSubscriptionClientFail
}

func (f *FailingSubscriptionClient) UniqueRequestID(ctx *resolve.Context, options GraphQLSubscriptionOptions, hash *xxhash.Digest) (err error) {
	return errSubscriptionClientFail
}

type testSubscriptionUpdater struct {
	updates []string
	done    bool
	mux     sync.Mutex
}

func (t *testSubscriptionUpdater) AwaitUpdates(tt *testing.T, timeout time.Duration, count int) {
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()
	for {
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ticker.C:
			tt.Fatalf("timed out waiting for updates")
		default:
			t.mux.Lock()
			if len(t.updates) == count {
				t.mux.Unlock()
				return
			}
			t.mux.Unlock()
		}
	}
}

func (t *testSubscriptionUpdater) AwaitDone(tt *testing.T, timeout time.Duration) {
	ticker := time.NewTicker(timeout)
	defer ticker.Stop()
	for {
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ticker.C:
			tt.Fatalf("timed out waiting for done")
		default:
			t.mux.Lock()
			if t.done {
				t.mux.Unlock()
				return
			}
			t.mux.Unlock()
		}
	}
}

func (t *testSubscriptionUpdater) Update(data []byte) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.updates = append(t.updates, string(data))
}

func (t *testSubscriptionUpdater) Done() {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.done = true
}

func TestSubscriptionSource_Start(t *testing.T) {
	chatServer := httptest.NewServer(subscriptiontesting.ChatGraphQLEndpointHandler())
	defer chatServer.Close()

	sendChatMessage := func(t *testing.T, username, message string) {
		time.Sleep(200 * time.Millisecond)
		httpClient := http.Client{}
		req, err := http.NewRequest(
			http.MethodPost,
			chatServer.URL,
			bytes.NewBufferString(fmt.Sprintf(`{"variables": {}, "operationName": "SendMessage", "query": "mutation SendMessage { post(roomName: \"#test\", username: \"%s\", text: \"%s\") { id } }"}`, username, message)),
		)
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	chatServerSubscriptionOptions := func(t *testing.T, body string) []byte {
		var gqlBody GraphQLBody
		_ = json.Unmarshal([]byte(body), &gqlBody)
		options := GraphQLSubscriptionOptions{
			URL:    chatServer.URL,
			Body:   gqlBody,
			Header: nil,
		}

		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)

		return optionsBytes
	}

	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		httpClient := http.Client{}
		subscriptionSource := SubscriptionSource{client: NewGraphQLSubscriptionClient(&httpClient, http.DefaultClient, ctx)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		source := SubscriptionSource{client: &FailingSubscriptionClient{}}
		err := source.Start(resolve.NewContext(context.Background()), []byte(`{"url": "", "body": "", "header": null}`), nil)
		assert.Error(t, err)
	})

	t.Run("should return error when subscription client returns an error", func(t *testing.T) {
		source := SubscriptionSource{client: &FailingSubscriptionClient{}}
		err := source.Start(resolve.NewContext(context.Background()), []byte(`{"url": "", "body": {}, "header": null}`), nil)
		assert.Error(t, err)
		assert.Equal(t, resolve.ErrUnableToResolve, err)
	})

	t.Run("invalid json: should stop before sending to upstream", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())
		defer ctx.Context().Done()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(ctx.Context())
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: "#test") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, updater)
		require.ErrorIs(t, err, resolve.ErrUnableToResolve)
	})

	t.Run("invalid syntax (roomNam)", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())
		defer ctx.Context().Done()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(ctx.Context())
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomNam: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, updater)
		require.NoError(t, err)
		updater.AwaitUpdates(t, time.Second, 1)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"errors":[{"message":"Unknown argument \"roomNam\" on field \"Subscription.messageAdded\". Did you mean \"roomName\"?","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"Field \"messageAdded\" argument \"roomName\" of type \"String!\" is required, but it was not provided.","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, updater.updates[0])
		updater.AwaitDone(t, time.Second)
	})

	t.Run("should close connection on stop message", func(t *testing.T) {
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(resolverLifecycle)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(resolve.NewContext(subscriptionLifecycle), chatSubscriptionOptions, updater)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		updater.AwaitUpdates(t, time.Second, 1)
		cancelSubscription()
		updater.AwaitDone(t, time.Second*5)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, updater.updates[0])
	})

	t.Run("should successfully subscribe with chat example", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())
		defer ctx.Context().Done()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(ctx.Context())
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, updater)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)
		updater.AwaitUpdates(t, time.Second, 1)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, updater.updates[0])
	})
}

func TestSubscription_GTWS_SubProtocol(t *testing.T) {
	chatServer := httptest.NewServer(subscriptiontesting.ChatGraphQLEndpointHandler())
	defer chatServer.Close()

	sendChatMessage := func(t *testing.T, username, message string) {
		time.Sleep(200 * time.Millisecond)
		httpClient := http.Client{}
		req, err := http.NewRequest(
			http.MethodPost,
			chatServer.URL,
			bytes.NewBufferString(fmt.Sprintf(`{"variables": {}, "operationName": "SendMessage", "query": "mutation SendMessage { post(roomName: \"#test\", username: \"%s\", text: \"%s\") { id } }"}`, username, message)),
		)
		require.NoError(t, err)

		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	chatServerSubscriptionOptions := func(t *testing.T, body string) []byte {
		var gqlBody GraphQLBody
		_ = json.Unmarshal([]byte(body), &gqlBody)
		options := GraphQLSubscriptionOptions{
			URL:    chatServer.URL,
			Body:   gqlBody,
			Header: nil,
		}

		optionsBytes, err := json.Marshal(options)
		require.NoError(t, err)

		return optionsBytes
	}

	newSubscriptionSource := func(ctx context.Context) SubscriptionSource {
		httpClient := http.Client{}
		subscriptionSource := SubscriptionSource{
			client: NewGraphQLSubscriptionClient(&httpClient, http.DefaultClient, ctx),
		}
		return subscriptionSource
	}

	t.Run("invalid syntax (roomNam)", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())
		defer ctx.Context().Done()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(ctx.Context())
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomNam: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, updater)
		require.NoError(t, err)

		updater.AwaitUpdates(t, time.Second, 1)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"errors":[{"message":"Unknown argument \"roomNam\" on field \"Subscription.messageAdded\". Did you mean \"roomName\"?","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"Field \"messageAdded\" argument \"roomName\" of type \"String!\" is required, but it was not provided.","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, updater.updates[0])
		updater.AwaitDone(t, time.Second)
		assert.Equal(t, true, updater.done)
	})

	t.Run("should close connection on stop message", func(t *testing.T) {
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(resolverLifecycle)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(resolve.NewContext(subscriptionLifecycle), chatSubscriptionOptions, updater)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		updater.AwaitUpdates(t, time.Second, 1)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, updater.updates[0])
		cancelSubscription()
		updater.AwaitDone(t, time.Second*5)
		assert.Equal(t, true, updater.done)
	})

	t.Run("should successfully subscribe with chat example", func(t *testing.T) {
		ctx := resolve.NewContext(context.Background())
		defer ctx.Context().Done()

		updater := &testSubscriptionUpdater{}

		source := newSubscriptionSource(ctx.Context())
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, updater)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		updater.AwaitUpdates(t, time.Second, 1)
		assert.Len(t, updater.updates, 1)
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, updater.updates[0])
	})
}

type runTestOnTestDefinitionOptions func(planConfig *plan.Configuration)

func runTestOnTestDefinition(t *testing.T, operation, operationName string, expectedPlan plan.Plan, options ...runTestOnTestDefinitionOptions) func(t *testing.T) {
	config := plan.Configuration{
		DataSources: []plan.DataSource{
			mustDataSourceConfigurationWithHttpClient(
				t,
				"ds-id",
				&plan.DataSourceMetadata{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"hero", "heroByBirthdate", "droid", "droids", "search", "stringList", "nestedStringList"},
						},
						{
							TypeName:   "Mutation",
							FieldNames: []string{"createReview"},
						},
						{
							TypeName:   "Subscription",
							FieldNames: []string{"remainingJedis"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"id", "stars", "commentary"},
						},
						{
							TypeName:   "Character",
							FieldNames: []string{"name", "friends"},
						},
						{
							TypeName:   "Human",
							FieldNames: []string{"name", "height", "friends"},
						},
						{
							TypeName:   "Droid",
							FieldNames: []string{"name", "primaryFunction", "friends"},
						},
						{
							TypeName:   "Starship",
							FieldNames: []string{"name", "length"},
						},
					},
				},
				mustCustomConfiguration(t, ConfigurationInput{
					Fetch: &FetchConfiguration{
						URL:    "https://swapi.com/graphql",
						Method: "POST",
					},
					Subscription: &SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
					SchemaConfiguration: mustSchema(t, nil, testDefinition),
				}),
			),
		},
		Fields: []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "heroByBirthdate",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "birthdate",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droid",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "id",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "droids",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "ids",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:       "name",
						SourceType: plan.FieldArgumentSource,
					},
				},
			},
		},
		DisableResolveFieldPositions: true,
	}

	return RunTest(testDefinition, operation, operationName, expectedPlan, config)
}

func TestSource_Load(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_, _ = fmt.Fprint(w, string(body))
	}))
	defer ts.Close()

	t.Run("unnull_variables", func(t *testing.T) {
		var (
			src       = &Source{httpClient: &http.Client{}}
			serverUrl = ts.URL
			variables = []byte(`{"a": null, "b": "b", "c": {}}`)
		)

		t.Run("should remove null variables and empty objects when flag is set", func(t *testing.T) {
			var input []byte
			input = httpclient.SetInputBodyWithPath(input, variables, "variables")
			input = httpclient.SetInputURL(input, []byte(serverUrl))
			input = httpclient.SetInputFlag(input, httpclient.UNNULL_VARIABLES)
			buf := bytes.NewBuffer(nil)

			require.NoError(t, src.Load(context.Background(), input, buf))
			assert.Equal(t, `{"variables":{"b":"b"}}`, buf.String())
		})

		t.Run("should only compact variables when no flag set", func(t *testing.T) {
			var input []byte
			input = httpclient.SetInputBodyWithPath(input, variables, "variables")
			input = httpclient.SetInputURL(input, []byte(serverUrl))

			buf := bytes.NewBuffer(nil)

			require.NoError(t, src.Load(context.Background(), input, buf))
			assert.Equal(t, `{"variables":{"a":null,"b":"b","c":{}}}`, buf.String())
		})
	})
	t.Run("remove undefined variables", func(t *testing.T) {
		var (
			src       = &Source{httpClient: &http.Client{}}
			serverUrl = ts.URL
			variables = []byte(`{"a":null,"b":null, "c": null}`)
		)
		t.Run("should remove undefined variables and leave null variables", func(t *testing.T) {
			var input []byte
			input = httpclient.SetInputBodyWithPath(input, variables, "variables")
			input = httpclient.SetInputURL(input, []byte(serverUrl))
			buf := bytes.NewBuffer(nil)

			undefinedVariables := []string{"a", "c"}
			ctx := context.Background()
			var err error
			input, err = httpclient.SetUndefinedVariables(input, undefinedVariables)
			assert.NoError(t, err)

			require.NoError(t, src.Load(ctx, input, buf))
			assert.Equal(t, `{"variables":{"b":null}}`, buf.String())
		})
	})
}

func TestUnNullVariables(t *testing.T) {
	t.Run("should not unnull variables if not enabled", func(t *testing.T) {
		t.Run("two variables, one null", func(t *testing.T) {
			s := &Source{}
			out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":null,"b":true}}}`))
			expected := `{"body":{"variables":{"a":null,"b":true}}}`
			assert.Equal(t, expected, string(out))
		})
	})

	t.Run("variables with whitespace and empty objects", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"email":null,"firstName": "FirstTest",		"lastName":"LastTest","phone":123456,"preferences":{ "notifications":{}},"password":"password"}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"firstName":"FirstTest","lastName":"LastTest","phone":123456,"password":"password"}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("empty variables", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("null inside an array", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"list":["a",null,"b"]}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"list":["a",null,"b"]}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("complex null inside nested objects and arrays", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":null, "b": {"key":null, "nested": {"nestedkey": null}}, "arr": ["1", null, "3"], "d": {"nested_arr":["4",null,"6"]}}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"b":{"key":null,"nested":{"nestedkey":null}},"arr":["1",null,"3"],"d":{"nested_arr":["4",null,"6"]}}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("two variables, one null", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":null,"b":true}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"b":true}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("two variables, one null reverse", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"a":true,"b":null}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"a":true}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("null variables", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":null},"unnull_variables":true}`))
		expected := `{"body":{"variables":null},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("ignore null inside non variables", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"foo":null},"body":"query {foo(bar: null){baz}}"},"unnull_variables":true}`))
		expected := `{"body":{"variables":{},"body":"query {foo(bar: null){baz}}"},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("ignore null in variable name", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"variables":{"not_null":1,"null":2,"not_null2":3}},"unnull_variables":true}`))
		expected := `{"body":{"variables":{"not_null":1,"null":2,"not_null2":3}},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("variables missing", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"query":"{foo}"},"unnull_variables":true}`))
		expected := `{"body":{"query":"{foo}"},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})

	t.Run("variables null", func(t *testing.T) {
		s := &Source{}
		out := s.compactAndUnNullVariables([]byte(`{"body":{"query":"{foo}","variables":null},"unnull_variables":true}`))
		expected := `{"body":{"query":"{foo}","variables":null},"unnull_variables":true}`
		assert.Equal(t, expected, string(out))
	})
}

const interfaceSelectionSchema = `

scalar String
scalar Boolean

schema {
	query: Query
}

type Query {
	user: User
}

interface User {
    id: ID!
    displayName: String!
    isLoggedIn: Boolean!
}

type RegisteredUser implements User {
    id: ID!
    displayName: String!
    isLoggedIn: Boolean!
	hasVerifiedEmail: Boolean!
}
`

const variableSchema = `

scalar String

schema {
	query: Query
}

type Query {
	user(name: String!): User
}

type User {
    normalized(data: NormalizedDataInput!): String!
}

input NormalizedDataInput {
    name: String!
}
`

const starWarsSchema = `
union SearchResult = Human | Droid | Starship

directive @format on FIELD
directive @onOperation on QUERY
directive @onVariable on VARIABLE_DEFINITION

scalar JSON

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    review(id: ID!): Review
    search(name: String!): SearchResult
    searchWithInput(input: SearchInput!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

input SearchInput {
	name: String
	options: JSON
}

type Mutation {
	createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    id: ID!
    name: String!
    friends: [Character]
}

type Human implements Character {
    id: ID!
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    id: ID!
    name: String!
    primaryFunction: String!
    friends: [Character]
}

type Starship {
    name: String!
    length: Float!
}`

const starWarsSchemaWithRenamedArgument = `
union SearchResult = Human | Droid | Starship

directive @format on FIELD
directive @onOperation on QUERY
directive @onVariable on VARIABLE_DEFINITION

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

scalar ReviewID

type Query {
    hero: Character
    droid(id: ID!): Droid
    review(id: ReviewID!): Review
    search(name: String!): SearchResult
    searchWithInput(input: SearchInput!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

input SearchInput {
	name: String
}

type Mutation {
	createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    name: String!
    friends: [Character]
}

type Human implements Character {
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    name: String!
    primaryFunction: String!
    friends: [Character]
}

type Starship {
    name: String!
    length: Float!
}`

const starWarsSchemaWithExportDirective = `
union SearchResult = Human | Droid | Starship

directive @format on FIELD
directive @onOperation on QUERY
directive @onVariable on VARIABLE_DEFINITION

directive @export (
	as: String!
) on FIELD

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
    search(name: String!): SearchResult
    searchWithInput(input: SearchInput!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

input SearchInput {
	name: String
}

type Mutation {
	createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    id: ID!
    name: String!
    friends: [Character]
}

type Human implements Character {
    id: ID!
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    id: ID!
    name: String!
    primaryFunction: String!
    friends: [Character]
}

type Starship {
    name: String!
    length: Float!
}`

const renamedStarWarsSchema = `
union SearchResult_api = Human_api | Droid_api | Starship_api

directive @api_format on FIELD
directive @otherapi_undefined on QUERY
directive @api_onOperation on QUERY
directive @api_onVariable on VARIABLE_DEFINITION

scalar JSON_api

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    api_hero: Character_api
    api_droid(id: ID!): Droid_api
    api_search(name: String!): SearchResult_api
    api_searchWithInput(input: SearchInput_api!): SearchResult_api
	api_stringList: [String]
	api_nestedStringList: [String]
}

input SearchInput_api {
	name: String
	options: JSON_api
}

type Mutation {
	createReview(episode: Episode_api!, review: ReviewInput_api!): Review_api
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput_api {
    stars: Int!
    commentary: String
}

type Review_api {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode_api {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character_api {
    name: String!
    friends: [Character_api]
}

type Human_api implements Character_api {
    name: String!
    height: String!
    friends: [Character_api]
}

type Droid_api implements Character_api {
    name: String!
    primaryFunction: String!
    friends: [Character_api]
}

type Starship_api {
    name: String!
    length: Float!
}`

const todoSchema = `

schema {
	query: Query
	mutation: Mutation
}

scalar ID
scalar String
scalar Boolean

""""""
scalar DateTime

""""""
enum DgraphIndex {
  """"""
  int
  """"""
  float
  """"""
  bool
  """"""
  hash
  """"""
  exact
  """"""
  term
  """"""
  fulltext
  """"""
  trigram
  """"""
  regexp
  """"""
  year
  """"""
  month
  """"""
  day
  """"""
  hour
}

""""""
input DateTimeFilter {
  """"""
  eq: DateTime
  """"""
  le: DateTime
  """"""
  lt: DateTime
  """"""
  ge: DateTime
  """"""
  gt: DateTime
}

""""""
input StringHashFilter {
  """"""
  eq: String
}

""""""
type UpdateTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type Subscription {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
input FloatFilter {
  """"""
  eq: Float
  """"""
  le: Float
  """"""
  lt: Float
  """"""
  ge: Float
  """"""
  gt: Float
}

""""""
input StringTermFilter {
  """"""
  allofterms: String
  """"""
  anyofterms: String
}

""""""
type DeleteTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
type Mutation {
  """"""
  addTask(input: [AddTaskInput!]!): AddTaskPayload
  """"""
  updateTask(input: UpdateTaskInput!): UpdateTaskPayload
  """"""
  deleteTask(filter: TaskFilter!): DeleteTaskPayload
  """"""
  addUser(input: [AddUserInput!]!): AddUserPayload
  """"""
  updateUser(input: UpdateUserInput!): UpdateUserPayload
  """"""
  deleteUser(filter: UserFilter!): DeleteUserPayload
}

""""""
enum HTTPMethod {
  """"""
  GET
  """"""
  POST
  """"""
  PUT
  """"""
  PATCH
  """"""
  DELETE
}

""""""
type DeleteUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  msg: String
  """"""
  numUids: Int
}

""""""
input TaskFilter {
  """"""
  id: [ID!]
  """"""
  title: StringFullTextFilter
  """"""
  completed: Boolean
  """"""
  and: TaskFilter
  """"""
  or: TaskFilter
  """"""
  not: TaskFilter
}

""""""
type UpdateUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
input TaskRef {
  """"""
  id: ID
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserFilter {
  """"""
  username: StringHashFilter
  """"""
  name: StringExactFilter
  """"""
  and: UserFilter
  """"""
  or: UserFilter
  """"""
  not: UserFilter
}

""""""
input UserOrder {
  """"""
  asc: UserOrderable
  """"""
  desc: UserOrderable
  """"""
  then: UserOrder
}

""""""
input AuthRule {
  """"""
  and: [AuthRule]
  """"""
  or: [AuthRule]
  """"""
  not: AuthRule
  """"""
  rule: String
}

""""""
type AddTaskPayload {
  """"""
  task(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  numUids: Int
}

""""""
type AddUserPayload {
  """"""
  user(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
  """"""
  numUids: Int
}

""""""
type Task {
  """"""
  id: ID!
  """"""
  title: String!
  """"""
  completed: Boolean!
  """"""
  user(filter: UserFilter): User!
}

""""""
input IntFilter {
  """"""
  eq: Int
  """"""
  le: Int
  """"""
  lt: Int
  """"""
  ge: Int
  """"""
  gt: Int
}

""""""
input StringExactFilter {
  """"""
  eq: String
  """"""
  le: String
  """"""
  lt: String
  """"""
  ge: String
  """"""
  gt: String
}

""""""
enum UserOrderable {
  """"""
  username
  """"""
  name
}

""""""
input AddTaskInput {
  """"""
  titleSets: [[String!]]
  """"""
  completed: Boolean!
  """"""
  user: UserRef!
}

""""""
input TaskPatch {
  """"""
  title: String
  """"""
  completed: Boolean
  """"""
  user: UserRef
}

""""""
input UserRef {
  """"""
  username: String
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input StringFullTextFilter {
  """"""
  alloftext: String
  """"""
  anyoftext: String
}

""""""
enum TaskOrderable {
  """"""
  title
}

""""""
input UpdateTaskInput {
  """"""
  filter: TaskFilter!
  """"""
  set: TaskPatch
  """"""
  remove: TaskPatch
}

""""""
input UserPatch {
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
type Query {
  """"""
  getTask(id: ID!): Task
  """"""
  queryTask(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
  """"""
  getUser(username: String!): User
  """"""
  queryUser(filter: UserFilter, order: UserOrder, first: Int, offset: Int): [User]
}

""""""
type User {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks(filter: TaskFilter, order: TaskOrder, first: Int, offset: Int): [Task]
}

""""""
enum Mode {
  """"""
  BATCH
  """"""
  SINGLE
}

""""""
input CustomHTTP {
  """"""
  url: String!
  """"""
  method: HTTPMethod!
  """"""
  body: String
  """"""
  graphql: String
  """"""
  mode: Mode
  """"""
  forwardHeaders: [String!]
  """"""
  secretHeaders: [String!]
  """"""
  introspectionHeaders: [String!]
  """"""
  skipIntrospection: Boolean
}

""""""
input StringRegExpFilter {
  """"""
  regexp: String
}

""""""
input AddUserInput {
  """"""
  username: String!
  """"""
  name: String
  """"""
  tasks: [TaskRef]
}

""""""
input TaskOrder {
  """"""
  asc: TaskOrderable
  """"""
  desc: TaskOrderable
  """"""
  then: TaskOrder
}

""""""
input UpdateUserInput {
  """"""
  filter: UserFilter!
  """"""
  set: UserPatch
  """"""
  remove: UserPatch
}
"""
The @cache directive caches the response server side and sets cache control headers according to the configuration.
With this setting you can reduce the load on your backend systems for operations that get hit a lot while data doesn't change that frequently. 
"""
directive @cache(
  """maxAge defines the maximum time in seconds a response will be understood 'fresh', defaults to 300 (5 minutes)"""
  maxAge: Int! = 300
  """
  vary defines the headers to append to the cache key
  In addition to all possible headers you can also select a custom claim for authenticated requests
  Examples: 'jwt.sub', 'jwt.team' to vary the cache key based on 'sub' or 'team' fields on the jwt. 
  """
  vary: [String]! = []
) on QUERY

"""The @auth directive lets you configure auth for a given operation"""
directive @auth(
  """disable explicitly disables authentication for the annotated operation"""
  disable: Boolean! = false
) on QUERY | MUTATION | SUBSCRIPTION

"""The @fromClaim directive overrides a variable from a select claim in the jwt"""
directive @fromClaim(
  """
  name is the name of the claim you want to use for the variable
  examples: sub, team, custom.nested.claim
  """
  name: String!
) on VARIABLE_DEFINITION
`

const testDefinition = `
union SearchResult = Human | Droid | Starship
scalar Date

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
	heroByBirthdate(birthdate: Date): Character
    droid(id: ID!): Droid
	droids(ids: [ID!]!): [Droid]
    search(name: String!): SearchResult
	stringList: [String]
	nestedStringList: [String]
}

type Mutation {
	createReview(episode: Episode!, review: ReviewInput!): Review
}

type Subscription {
    remainingJedis: Int!
}

input ReviewInput {
    stars: Int!
    commentary: String
}

type Review {
    id: ID!
    stars: Int!
    commentary: String
}

enum Episode {
    NEWHOPE
    EMPIRE
    JEDI
}

interface Character {
    name: String!
    friends: [Character]
}

type Human implements Character {
    name: String!
    height: String!
    friends: [Character]
}

type Droid implements Character {
    name: String!
    primaryFunction: String!
    friends: [Character]
}

type Starship {
    name: String!
    length: Float!
}`

const federationTestSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Product {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review]
}

type Query {
  me: User
  topProducts(first: Int = 5): [Product]
}

type Review {
  body: String!
  author: User!
  product: Product!
  notes: String
  likes(filterToPublicOnly: Boolean): Int!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`

const federationTestSchemaWithRename = `
scalar String
scalar Int
scalar ID
scalar XBoolean

schema {
	query: Query
}

type Product {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review]
}

type Query {
  me: User
  topProducts(first: Int = 5): [Product]
}

type Review {
  body: String!
  author: User!
  product: Product!
  notes: String
  likes(filterToPublicOnly: XBoolean!): Int!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`

const complexFederationSchema = `
scalar String
scalar Int
scalar Float
scalar ID

union AccountType = PasswordAccount | SMSAccount
type Amazon {
  referrer: String
}

union Brand = Ikea | Amazon
type Car implements Vehicle {
  id: String!
  description: String
  price: String
}

type Error {
  code: Int
  message: String
}

type Furniture implements Product {
  upc: String!
  sku: String!
  name: String
  price: String
  brand: Brand
  metadata: [MetadataOrError]
  details: ProductDetailsFurniture
  inStock: Int!
}

type Ikea {
  asile: Int
}

type KeyValue {
  key: String!
  value: String!
}

union MetadataOrError = KeyValue | Error
type Mutation {
  login(username: String!, password: String!): User
}

type Name {
  first: String
  last: String
}

type PasswordAccount {
  email: String!
}

interface Product {
  upc: String!
  sku: String!
  name: String
  price: String
  details: ProductDetails
  inStock: Int!
}

interface ProductDetails {
  country: String
}

type ProductDetailsBook implements ProductDetails {
  country: String
  pages: Int
}

type ProductDetailsFurniture implements ProductDetails {
  country: String
  color: String
}

type Query {
  me: User
  user(id: ID!): User
  product(upc: String!): Product
  vehicle(id: String!): Vehicle
  topProducts(first: Int = 5): [Product]
  topCars(first: Int = 5): [Car]
}

type Review {
  body: String!
  author: User!
  product: Product!
}

type SMSAccount {
  number: String
}

type Subscription {
  updatedPrice: Product!
  updateProductPrice(upc: String!): Product!
  stock: [Product!]
}

union Thing = Car | Ikea
type User {
  id: ID!
  name: Name
  username: String
  birthDate(locale: String): String
  account: AccountType
  metadata: [UserMetadata]
  ssn: String
  vehicle: Vehicle
  thing: Thing
  reviews: [Review]
}

type UserMetadata {
  name: String
  address: String
  description: String
}

type Van implements Vehicle {
  id: String!
  description: String
  price: String
}

interface Vehicle {
  id: String!
  description: String
  price: String
}
`

const renamedFederationTestSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Product_api {
  upc: String!
  name: String!
  price: Int!
  reviews: [Review_api]
}

type Query {
  api_me: User_api
  api_topProducts(first: Int = 5): [Product_api]
}

type Review_api {
  body: String!
  author: User_api!
  product: Product_api!
}

type User_api {
  id: ID!
  username: String!
  reviews: [Review_api]
}
`

const requiredFieldTestSchema = `
scalar String
scalar ID

schema {
	query: Query
}

type Query {
  serviceOne: ServiceOneType
}

type ServiceOneType {
  id: ID!
  serviceOneFieldOne: String!
  serviceOneFieldTwo: String!

  serviceTwoFieldOne: String!
  serviceTwoFieldTwo: String!
}
`

const subgraphTestSchema = `
directive @external on FIELD_DEFINITION
directive @requires(fields: _FieldSet!) on FIELD_DEFINITION
directive @provides(fields: _FieldSet!) on FIELD_DEFINITION
directive @key(fields: _FieldSet!) on OBJECT | INTERFACE
directive @extends on OBJECT

scalar _Any
union _Entity = Product | User
scalar _FieldSet

type _Service {
  sdl: String
}

type Entity {
  findProductByUpc(upc: String!): Product!
  findUserByID(id: ID!): User!
}

type Product {
  upc: String!
  reviews: [Review]
}

type Query {
  _entities(representations: [_Any!]!): [_Entity]!
  _service: _Service!
}

type Review {
  body: String!
  author: User!
  product: Product!
}

type User {
  id: ID!
  username: String!
  reviews: [Review]
}
`

const countriesSchema = `
scalar String
scalar Int
scalar ID

schema {
	query: Query
}

type Country {
  name: String!
  code: ID!
}

type Query {
  country(code: ID!): Country
  countryAlias(code: ID!): Country
}
`

const wgSchema = `union DeleteEnvironmentResponse = Success | Error

type Query {
  user: User
  edges: [Edge!]!
  admin_Config: AdminConfigResponse!
}

type NamespaceMemberRemoved {
  namespace: Namespace!
}

type NamespaceMemberAdded {
  namespace: Namespace!
}

union DeleteNamespaceResponse = Success | Error

enum Membership {
  owner
  maintainer
  viewer
  guest
}

input CreateAccessToken {
  name: String!
}

type ApiCreated {
  api: Api!
}

scalar Time

union NamespaceRemoveMemberResponse = NamespaceMemberRemoved | Error

enum EnvironmentKind {
  Personal
  Team
  Business
}

type Edge {
  id: ID!
  name: String!
  location: String!
}

type NamespaceCreated {
  namespace: Namespace!
}

union UpdateEnvironmentResponse = EnvironmentUpdated | Error

type Deployment {
  id: ID!
  name: String!
  config: JSON!
  environments: [Environment!]!
}

type Error {
  code: ErrorCode!
  message: String!
}

type Mutation {
  accessTokenCreate(input: CreateAccessToken!): CreateAccessTokenResponse!
  accessTokenDelete(input: DeleteAccessToken!): DeleteAccessTokenResponse!
  apiCreate(input: CreateApi!): CreateApiResponse!
  apiUpdate(input: UpdateApi!): UpdateApiResponse!
  apiDelete(input: DeleteApi!): DeleteApiResponse!
  deploymentCreateOrUpdate(input: CreateOrUpdateDeployment!): CreateOrUpdateDeploymentResponse!
  deploymentDelete(input: DeleteDeployment!): DeleteDeploymentResponse!
  environmentCreate(input: CreateEnvironment!): CreateEnvironmentResponse!
  environmentUpdate(input: UpdateEnvironment!): UpdateEnvironmentResponse!
  environmentDelete(input: DeleteEnvironment!): DeleteEnvironmentResponse!
  namespaceCreate(input: CreateNamespace!): CreateNamespaceResponse!
  namespaceDelete(input: DeleteNamespace!): DeleteNamespaceResponse!
  namespaceAddMember(input: NamespaceAddMember!): NamespaceAddMemberResponse!
  namespaceRemoveMember(input: NamespaceRemoveMember!): NamespaceRemoveMemberResponse!
  namespaceUpdateMembership(input: NamespaceUpdateMembership!): NamespaceUpdateMembershipResponse!
  admin_setWunderNodeImageTag(imageTag: String!): AdminConfigResponse!
}

type AccessToken {
  id: ID!
  name: String!
  createdAt: Time!
}

type EnvironmentCreated {
  environment: Environment!
}

type DeploymentUpdated {
  deployment: Deployment!
}

enum ErrorCode {
  Internal
  AuthenticationRequired
  Unauthorized
  NotFound
  Conflict
  UserAlreadyHasPersonalNamespace
  TeamPlanInPersonalNamespace
  InvalidName
  UnableToDeployEnvironment
  InvalidWunderGraphConfig
  ApiEnvironmentNamespaceMismatch
  UnableToUpdateEdgesOnPersonalEnvironment
}

input CreateEnvironment {
  namespace: ID!
  name: String
  primary: Boolean!
  kind: EnvironmentKind!
  edges: [ID!]
}

type Environment {
  id: ID!
  name: String
  primary: Boolean!
  kind: EnvironmentKind!
  edges: [Edge!]
  primaryHostName: String!
  hostNames: [String!]!
}

type DeploymentCreated {
  deployment: Deployment!
}

union AdminConfigResponse = Error | AdminConfig

input CreateNamespace {
  name: String!
  personal: Boolean!
}

input NamespaceUpdateMembership {
  namespaceID: ID!
  memberID: ID!
  newMembership: Membership!
}

union DeleteApiResponse = Success | Error

type ApiUpdated {
  api: Api!
}

input DeleteDeployment {
  deploymentID: ID!
}

input NamespaceRemoveMember {
  namespaceID: ID!
  memberID: ID!
}

union NamespaceUpdateMembershipResponse = NamespaceMembershipUpdated | Error

type User {
  id: ID!
  name: String!
  email: String!
  namespaces: [Namespace!]!
  accessTokens: [AccessToken!]!
}

input DeleteApi {
  id: ID!
}

type NamespaceMembershipUpdated {
  namespace: Namespace!
}

type EnvironmentUpdated {
  environment: Environment!
}

union CreateNamespaceResponse = NamespaceCreated | Error

type Namespace {
  id: ID!
  name: String!
  members: [Member!]!
  apis: [Api!]!
  environments: [Environment!]!
  personal: Boolean!
}

input UpdateEnvironment {
  environmentID: ID!
  edgeIDs: [ID!]
}

input DeleteEnvironment {
  environmentID: ID!
}

enum ApiVisibility {
  public
  private
  namespace
}

type Member {
  user: User!
  membership: Membership!
}

union DeleteAccessTokenResponse = Success | Error

input CreateApi {
  apiName: String!
  namespaceID: String!
  visibility: ApiVisibility!
  markdownDescription: String!
}

union CreateApiResponse = ApiCreated | Error

union CreateEnvironmentResponse = EnvironmentCreated | Error

union UpdateApiResponse = ApiUpdated | Error

input CreateOrUpdateDeployment {
  apiID: ID!
  name: String
  config: JSON!
  environmentIDs: [ID!]!
}

union CreateOrUpdateDeploymentResponse = DeploymentCreated | DeploymentUpdated | Error

union CreateAccessTokenResponse = AccessTokenCreated | Error

input DeleteAccessToken {
  id: ID!
}

type AdminConfig {
  WunderNodeImageTag: String!
}

input UpdateApi {
  id: ID!
  apiName: String!
  config: JSON!
  visibility: ApiVisibility!
  markdownDescription: String!
}

type Success {
  message: String!
}

scalar JSON

input NamespaceAddMember {
  namespaceID: ID!
  newMemberEmail: String!
  membership: Membership
}

input DeleteNamespace {
  namespaceID: ID!
}

type AccessTokenCreated {
  token: String!
  accessToken: AccessToken!
}

union NamespaceAddMemberResponse = NamespaceMemberAdded | Error

type Api {
  id: ID!
  name: String!
  visibility: ApiVisibility!
  deployments: [Deployment!]!
  markdownDescription: String!
}

union DeleteDeploymentResponse = Success | Error
`

const userSchema = `
type Query {
  user(id: ID!): User
}

type User {
  id: ID!
  name: String!
  tier: Tier
  meta: Map!
}

enum Tier {
  A
  B
  C
}

scalar Map
`

const customUserSchema = `
type Query {
  custom_user(id: ID!): custom_User
}

type custom_User {
  id: ID!
  name: String!
  tier: custom_Tier
  meta: custom_Meta!
}

enum custom_Tier {
  A
  B
  C
}

type custom_Meta {
  foo: String!
}

scalar custom_Map
`

const federatedSchemaWithInterfaceQuery = `
	scalar String
	scalar Int
	scalar ID
	
	schema {
		query: Query
	}
	
	type Query {
		me: User
		self: Identity
	}
	
	interface Identity {
		id: ID!
	}
	
	type User implements Identity {
		id: ID!
		username: String!
		reviews: [Review]
	}

	interface Medium {
		size: Int!
	}

	type Image implements Medium {
		size: Int!
		extension: String!
	}

	type Video implements Medium {
		size: Int!
		length: Int!
	}
	
	type Review {
		id: ID!
		body: String!
		author: User!
		attachment: Medium!
	}
`

const reviewSDL = `
	interface Medium {
		size: Int!
	}

	type Image implements Medium {
		size: Int!
		extension: String!
	}

	type Video implements Medium {
		size: Int!
		length: Int!
	}

	type Review @key(fields: "id") {
		id: ID!
		body: String!
		author: User! @provides(fields: "username")
		attachment: Medium
	}
	
	extend type User @key(fields: "id") {
		id: ID! @external
		reviews: [Review] 
	}
`

const userSDLWithInterface = `
	extend type Query {
		me: User
		self: Identity
	}
	
	interface Identity {
		id: ID!
	}
	
	type User implements Identity @key(fields: "id") {
		id: ID!
		username: String!
	}
`

const federatedSchemaWithComplexNestedFragments = `
	type Query {
		user: User
	}
	type User {
		id: ID!
		username: String!
		pets: [Pet!]!
	}
	type Details {
		hasOwner: Boolean!
		age: Int!
	}
	interface Pet {
		name: String!
		species: String!
		details: Details!
	}
	type Cat implements Pet {
		name: String!
		species: String!
		catField: String!
		details: Details!
	}
	type Dog implements Pet {
		name: String!
		species: String!
		dogField: String!
		details: Details!
	}
`

const simpleUserSchema = `
	extend type Query {
		user: User
	}
	type User @key(fields: "id") {
		id: ID!
		username: String!
	}
`

const petSchema = `
	type Details {
		age: Int!
		hasOwner : Boolean!
	}
	interface Pet {
		name: String!
		species: String!
		details: Details!
	}
	type Cat implements Pet {
		name: String!
		species: String!
		catField: String!
		details: Details!
	}
	type Dog implements Pet {
		name: String!
		species: String!
		dogField: String!
		details: Details!
	}
	extend type User @key(fields: "id") {
		id: ID! @external
		pets: [Pet!]!
	}
`
