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

	"github.com/buger/jsonparser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jensneuse/graphql-go-tools/examples/chat"
	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
)

func TestGraphQLDataSource(t *testing.T) {
	t.Run("simple named Query", RunTest(starWarsSchema, `
		query MyQuery($id: ID!){
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
					DataSource: &Source{},
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$1$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":$$0$$}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path:                 []string{"id"},
							JsonValueType:        jsonparser.String,
							RenderAsGraphQLValue: true,
						},
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					),
					DataSourceIdentifier:  []byte("graphql_datasource.Source"),
					ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("droid"),
						Position: resolve.Position{
							Line:   3,
							Column: 4,
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
									Position: resolve.Position{
										Line:   4,
										Column: 5,
									},
								},
								{
									Name: []byte("aliased"),
									Value: &resolve.String{
										Path: []string{"aliased"},
									},
									Position: resolve.Position{
										Line:   5,
										Column: 5,
									},
								},
								{
									Name: []byte("friends"),
									Position: resolve.Position{
										Line:   6,
										Column: 5,
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
													Position: resolve.Position{
														Line:   7,
														Column: 6,
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
									Position: resolve.Position{
										Line:   9,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("hero"),
						Position: resolve.Position{
							Line:   11,
							Column: 4,
						},
						Value: &resolve.Object{
							Path:     []string{"hero"},
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path: []string{"name"},
									},
									Position: resolve.Position{
										Line:   12,
										Column: 5,
									},
								},
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("stringList"),
						Position: resolve.Position{
							Line:   14,
							Column: 4,
						},
						Value: &resolve.Array{
							Nullable: true,
							Item: &resolve.String{
								Nullable: true,
							},
						},
					},
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("nestedStringList"),
						Position: resolve.Position{
							Line:   15,
							Column: 4,
						},
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
		DataSources: []plan.DataSourceConfiguration{
			{
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
				Factory: &Factory{},
				Custom: ConfigJson(Configuration{
					Fetch: FetchConfiguration{
						URL: "https://swapi.com/graphql",
						Header: http.Header{
							"Authorization":    []string{"{{ .request.headers.Authorization }}"},
							"Invalid-Template": []string{"{{ request.headers.Authorization }}"},
						},
					},
				}),
			},
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
	}))

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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"name"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("addFriend"),
							Position: resolve.Position{
								Line:   1,
								Column: 37,
							},
							Value: &resolve.Object{
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
										Position: resolve.Position{
											Line:   1,
											Column: 61,
										},
									},
									{
										Name: []byte("name"),
										Value: &resolve.String{
											Path: []string{"name"},
										},
										Position: resolve.Position{
											Line:   1,
											Column: 64,
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.one",
						},
					}),
					Factory: &Factory{},
				},
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://foo.service","body":{"query":"query($a: String, $b: String){foo(bar: $a){bar(bal: $b)}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"a"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"b"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   4,
											Column: 5,
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://foo.service",
						},
					}),
				},
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} alias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"a"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"b"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("alias"),
							Position: resolve.Position{
								Line:   6,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   7,
											Column: 5,
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://countries.service",
						},
					}),
				},
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} countryAlias: country(code: $b){name}}","variables":{"b":$$1$$,"a":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"a"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"b"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  false,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("countryAlias"),
							Position: resolve.Position{
								Line:   6,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   7,
											Column: 5,
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Factory: &Factory{},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://countries.service",
						},
					}),
				},
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
		},
	))

	nestedGraphQLEngineFactory := &Factory{}
	t.Run("nested graphql engines", RunTest(`
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
	`, `
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
								BufferId:   0,
								Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:                 []string{"firstArg"},
										JsonValueType:        jsonparser.String,
										RenderAsGraphQLValue: true,
									},
									&resolve.ContextVariable{
										Path:                 []string{"thirdArg"},
										JsonValueType:        jsonparser.Number,
										RenderAsGraphQLValue: true,
									},
								),
								DataSourceIdentifier:  []byte("graphql_datasource.Source"),
								ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
							},
							&resolve.SingleFetch{
								BufferId:   2,
								Input:      `{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo serviceOneField} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path:                 []string{"secondArg"},
										JsonValueType:        jsonparser.Boolean,
										RenderAsGraphQLValue: true,
									},
									&resolve.ContextVariable{
										Path:                 []string{"fourthArg"},
										JsonValueType:        jsonparser.Number,
										RenderAsGraphQLValue: true,
									},
								),
								DataSourceIdentifier:  []byte("graphql_datasource.Source"),
								ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
							},
						},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("serviceOne"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceOne"},

								Fetch: &resolve.SingleFetch{
									BufferId:              1,
									DataSource:            &Source{},
									Input:                 `{"method":"POST","url":"https://country.service","body":{"query":"{countries {name}}"}}`,
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},

								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
									},
									{
										Name:      []byte("countries"),
										HasBuffer: true,
										BufferID:  1,
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
										Value: &resolve.Array{
											Path: []string{"countries"},
											Item: &resolve.Object{
												Fields: []*resolve.Field{
													{
														Name: []byte("name"),
														Value: &resolve.String{
															Path: []string{"name"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 6,
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
							HasBuffer: true,
							BufferID:  2,
							Name:      []byte("serviceTwo"),
							Position: resolve.Position{
								Line:   9,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceTwo"},
								Fetch: &resolve.SingleFetch{
									BufferId:   3,
									DataSource: &Source{},
									Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOneResponse: serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":$$0$$}}}`,
									Variables: resolve.NewVariables(
										&resolve.ObjectVariable{
											Path:                 []string{"serviceOneField"},
											RenderAsGraphQLValue: true,
										},
									),
									DataSourceIdentifier:  []byte("graphql_datasource.Source"),
									ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
								},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldTwo"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"fieldTwo"},
										},
										Position: resolve.Position{
											Line:   10,
											Column: 5,
										},
									},
									{
										HasBuffer: true,
										BufferID:  3,
										Name:      []byte("serviceOneResponse"),
										Position: resolve.Position{
											Line:   11,
											Column: 5,
										},
										Value: &resolve.Object{
											Nullable: true,
											Path:     []string{"serviceOneResponse"},
											Fields: []*resolve.Field{
												{
													Name: []byte("fieldOne"),
													Value: &resolve.String{
														Path: []string{"fieldOne"},
													},
													Position: resolve.Position{
														Line:   12,
														Column: 6,
													},
												},
											},
										},
									},
								},
							},
						},
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("anotherServiceOne"),
							Position: resolve.Position{
								Line:   15,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"anotherServiceOne"},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   16,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  2,
							HasBuffer: true,
							Name:      []byte("secondServiceTwo"),
							Position: resolve.Position{
								Line:   18,
								Column: 4,
							},
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
										Position: resolve.Position{
											Line:   19,
											Column: 5,
										},
									},
									{
										Name: []byte("serviceOneField"),
										Value: &resolve.String{
											Path:     []string{"serviceOneField"},
											Nullable: true,
										},
										Position: resolve.Position{
											Line:   20,
											Column: 5,
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("reusingServiceOne"),
							Position: resolve.Position{
								Line:   22,
								Column: 4,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"reusingServiceOne"},
								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
										Position: resolve.Position{
											Line:   23,
											Column: 5,
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.one",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://service.two",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://country.service",
						},
					}),
					Factory: nestedGraphQLEngineFactory,
				},
			},
			Fields: []plan.FieldConfiguration{
				{
					TypeName:       "ServiceTwoResponse",
					FieldName:      "serviceOneResponse",
					Path:           []string{"serviceOne"},
					RequiresFields: []string{"serviceOneField"},
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
		},
	))

	t.Run("mutation with variables in array object argument", RunTest(
		todoSchema,
		`mutation AddTask($title: String!, $completed: Boolean!, $name: String! @fromClaim(name: "sub")) {
					  addTask(input: [{title: $title, completed: $completed, user: {name: $name}}]){
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://graphql.service","body":{"query":"mutation($title: String!, $completed: Boolean!, $name: String!){addTask(input: [{title: $title,completed: $completed,user: {name: $name}}]){task {id title completed}}}","variables":{"name":$$2$$,"completed":$$1$$,"title":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"title"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"completed"},
								JsonValueType:        jsonparser.Boolean,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"name"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("addTask"),
							Position: resolve.Position{
								Line:   2,
								Column: 8,
							},
							Value: &resolve.Object{
								Path:     []string{"addTask"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("task"),
										Position: resolve.Position{
											Line:   3,
											Column: 7,
										},
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
														Position: resolve.Position{
															Line:   4,
															Column: 9,
														},
													},
													{
														Name: []byte("title"),
														Value: &resolve.String{
															Path: []string{"title"},
														},
														Position: resolve.Position{
															Line:   5,
															Column: 9,
														},
													},
													{
														Name: []byte("completed"),
														Value: &resolve.Boolean{
															Path: []string{"completed"},
														},
														Position: resolve.Position{
															Line:   6,
															Column: 9,
														},
													},
												},
											},
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://graphql.service",
						},
					}),
					Factory: &Factory{},
				},
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
		},
	))

	t.Run("inline object value with arguments", RunTest(`
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
			`, `
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"https://user.service","body":{"query":"mutation($id: String, $name: String){createUser(input: {user: {id: $id,username: $name}}){user {id username createdDate}}}","variables":{"name":$$1$$,"id":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"id"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"name"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("createUser"),
							Position: resolve.Position{
								Line:   3,
								Column: 6,
							},
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"createUser"},
								Fields: []*resolve.Field{
									{
										Name: []byte("user"),
										Position: resolve.Position{
											Line:   4,
											Column: 5,
										},
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
													Position: resolve.Position{
														Line:   5,
														Column: 7,
													},
												},
												{
													Name: []byte("username"),
													Value: &resolve.String{
														Path:     []string{"username"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   6,
														Column: 7,
													},
												},
												{
													Name: []byte("createdDate"),
													Value: &resolve.String{
														Path:     []string{"createdDate"},
														Nullable: true,
													},
													Position: resolve.Position{
														Line:   7,
														Column: 7,
													},
												},
											},
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
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "https://user.service",
						},
					}),
					Factory: &Factory{},
				},
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
		},
	))

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
						BufferId:   0,
						Input:      `{"method":"POST","url":"http://api.com","body":{"query":"mutation($name: String!, $personal: Boolean!){__typename namespaceCreate(input: {name: $name,personal: $personal}){__typename ... on NamespaceCreated {namespace {id name}} ... on Error {code message}}}","variables":{"personal":$$1$$,"name":$$0$$}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path:                 []string{"name"},
								JsonValueType:        jsonparser.String,
								RenderAsGraphQLValue: true,
							},
							&resolve.ContextVariable{
								Path:                 []string{"personal"},
								JsonValueType:        jsonparser.Boolean,
								RenderAsGraphQLValue: true,
							},
						),
						DisallowSingleFlight:  true,
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__typename"),
							Value: &resolve.String{
								Path:     []string{"__typename"},
								Nullable: false,
							},
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
						},
						{
							Name:      []byte("namespaceCreate"),
							HasBuffer: true,
							BufferID:  0,
							Position: resolve.Position{
								Line:   4,
								Column: 4,
							},
							Value: &resolve.Object{
								Path: []string{"namespaceCreate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("__typename"),
										Value: &resolve.String{
											Path:     []string{"__typename"},
											Nullable: false,
										},
										Position: resolve.Position{
											Line:   5,
											Column: 5,
										},
									},
									{
										OnTypeName: []byte("NamespaceCreated"),
										Name:       []byte("namespace"),
										Position: resolve.Position{
											Line:   7,
											Column: 6,
										},
										Value: &resolve.Object{
											Path: []string{"namespace"},
											Fields: []*resolve.Field{
												{
													Name: []byte("id"),
													Value: &resolve.String{
														Path:     []string{"id"},
														Nullable: false,
													},
													Position: resolve.Position{
														Line:   8,
														Column: 7,
													},
												},
												{
													Name: []byte("name"),
													Value: &resolve.String{
														Path:     []string{"name"},
														Nullable: false,
													},
													Position: resolve.Position{
														Line:   9,
														Column: 7,
													},
												},
											},
										},
									},
									{
										OnTypeName: []byte("Error"),
										Name:       []byte("code"),
										Value: &resolve.String{
											Path: []string{"code"},
										},
										Position: resolve.Position{
											Line:   13,
											Column: 6,
										},
									},
									{
										OnTypeName: []byte("Error"),
										Name:       []byte("message"),
										Value: &resolve.String{
											Path: []string{"message"},
										},
										Position: resolve.Position{
											Line:   14,
											Column: 6,
										},
									},
								},
							},
						},
					},
				},
			},
		}, plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{
				{
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
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL:    "http://api.com",
							Method: "POST",
						},
						Subscription: SubscriptionConfiguration{
							URL: "ws://api.com",
						},
					}),
					Factory: &Factory{},
				},
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
			DefaultFlushIntervalMillis: 500,
		}))
	factory := &Factory{
		HTTPClient: http.DefaultClient,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Run("subscription", RunTest(testDefinition, `
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				Input: []byte(`{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`),
				Source: &SubscriptionSource{
					NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("remainingJedis"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
							Value: &resolve.Integer{
								Path:     []string{"remainingJedis"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}, plan.Configuration{
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"remainingJedis"},
					},
				},
				Custom: ConfigJson(Configuration{
					Subscription: SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
				Factory: factory,
			},
		},
	}))

	t.Run("subscription with variables", RunTest(`
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
						Path:                 []string{"a"},
						JsonValueType:        jsonparser.String,
						RenderAsGraphQLValue: true,
					},
				),
				Source: &SubscriptionSource{
					client: NewWebSocketGraphQLSubscriptionClient(http.DefaultClient, ctx),
				},
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fields: []*resolve.Field{
						{
							Name: []byte("foo"),
							Position: resolve.Position{
								Line:   3,
								Column: 4,
							},
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
		DataSources: []plan.DataSourceConfiguration{
			{
				RootNodes: []plan.TypeField{
					{
						TypeName:   "Subscription",
						FieldNames: []string{"foo"},
					},
				},
				Custom: ConfigJson(Configuration{
					Subscription: SubscriptionConfiguration{
						URL: "wss://swapi.com/graphql",
					},
				}),
				Factory: factory,
			},
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
	}))

	batchFactory := NewBatchFactory()
	federationFactory := &Factory{BatchFactory: batchFactory}
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
						BufferId:              0,
						Input:                 `{"method":"POST","url":"http://user.service","body":{"query":"{me {id username}}"}}`,
						DataSource:            &Source{},
						DataSourceIdentifier:  []byte("graphql_datasource.Source"),
						ProcessResponseConfig: resolve.ProcessResponseConfig{ExtractGraphqlResponse: true},
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("me"),
							Position: resolve.Position{
								Line:   2,
								Column: 7,
							},
							Value: &resolve.Object{
								Fetch: &resolve.BatchFetch{
									Fetch: &resolve.SingleFetch{
										BufferId: 1,
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body author {id username} product {upc}}}}}","variables":{"representations":[{"id":$$0$$,"__typename":"User"}]}}}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path:                 []string{"id"},
												JsonValueType:        jsonparser.String,
												RenderAsGraphQLValue: true,
											},
										),
										DataSource:           &Source{},
										DataSourceIdentifier: []byte("graphql_datasource.Source"),
										ProcessResponseConfig: resolve.ProcessResponseConfig{
											ExtractGraphqlResponse:    true,
											ExtractFederationEntities: true,
										},
									},
									BatchFactory: batchFactory,
								},
								Path:     []string{"me"},
								Nullable: true,
								Fields: []*resolve.Field{
									{
										Name: []byte("id"),
										Value: &resolve.String{
											Path: []string{"id"},
										},
										Position: resolve.Position{
											Line:   3,
											Column: 8,
										},
									},
									{
										Name: []byte("username"),
										Value: &resolve.String{
											Path: []string{"username"},
										},
										Position: resolve.Position{
											Line:   4,
											Column: 8,
										},
									},
									{
										HasBuffer: true,
										BufferID:  1,
										Name:      []byte("reviews"),
										Position: resolve.Position{
											Line:   5,
											Column: 8,
										},
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
														Position: resolve.Position{
															Line:   6,
															Column: 9,
														},
													},
													{
														Name: []byte("author"),
														Position: resolve.Position{
															Line:   7,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"author"},
															Fields: []*resolve.Field{
																{
																	Name: []byte("id"),
																	Value: &resolve.String{
																		Path: []string{"id"},
																	},
																	Position: resolve.Position{
																		Line:   8,
																		Column: 10,
																	},
																},
																{
																	Name: []byte("username"),
																	Value: &resolve.String{
																		Path: []string{"username"},
																	},
																	Position: resolve.Position{
																		Line:   9,
																		Column: 10,
																	},
																},
															},
														},
													},
													{
														Name: []byte("product"),
														Position: resolve.Position{
															Line:   11,
															Column: 9,
														},
														Value: &resolve.Object{
															Path: []string{"product"},
															Fetch: &resolve.ParallelFetch{
																Fetches: []resolve.Fetch{
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId:   2,
																			Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"name":$$1$$,"upc":$$0$$,"__typename":"Product"}]}}}`,
																			DataSource: &Source{},
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:                 []string{"upc"},
																					JsonValueType:        jsonparser.String,
																					RenderAsGraphQLValue: true,
																				},
																				&resolve.ObjectVariable{
																					Path:                 []string{"name"},
																					JsonValueType:        jsonparser.String,
																					RenderAsGraphQLValue: true,
																				},
																			),
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId: 3,
																			Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {id username}}}}}","variables":{"representations":[{"name":$$1$$,"upc":$$0$$,"__typename":"Product"}]}}}`,
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path:                 []string{"upc"},
																					JsonValueType:        jsonparser.String,
																					RenderAsGraphQLValue: true,
																				},
																				&resolve.ObjectVariable{
																					Path:                 []string{"name"},
																					JsonValueType:        jsonparser.String,
																					RenderAsGraphQLValue: true,
																				},
																			),
																			DataSource:           &Source{},
																			DataSourceIdentifier: []byte("graphql_datasource.Source"),
																			ProcessResponseConfig: resolve.ProcessResponseConfig{
																				ExtractGraphqlResponse:    true,
																				ExtractFederationEntities: true,
																			},
																		},
																		BatchFactory: batchFactory,
																	},
																},
															},
															Fields: []*resolve.Field{
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("name"),
																	Value: &resolve.String{
																		Path: []string{"name"},
																	},
																	Position: resolve.Position{
																		Line:   12,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																	Position: resolve.Position{
																		Line:   13,
																		Column: 10,
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  3,
																	Name:      []byte("reviews"),
																	Position: resolve.Position{
																		Line:   14,
																		Column: 10,
																	},
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
																					Position: resolve.Position{
																						Line:   15,
																						Column: 11,
																					},
																				},
																				{
																					Name: []byte("author"),
																					Position: resolve.Position{
																						Line:   16,
																						Column: 11,
																					},
																					Value: &resolve.Object{
																						Path: []string{"author"},
																						Fields: []*resolve.Field{
																							{
																								Name: []byte("id"),
																								Value: &resolve.String{
																									Path: []string{"id"},
																								},
																								Position: resolve.Position{
																									Line:   17,
																									Column: 12,
																								},
																							},
																							{
																								Name: []byte("username"),
																								Value: &resolve.String{
																									Path: []string{"username"},
																								},
																								Position: resolve.Position{
																									Line:   18,
																									Column: 12,
																								},
																							},
																						},
																					},
																				},
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
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
			DataSources: []plan.DataSourceConfiguration{
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "Query",
							FieldNames: []string{"me"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://user.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {me: User} type User @key(fields: \"id\"){ id: ID! username: String!}",
						},
					}),
					Factory: federationFactory,
				},
				{
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
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Product",
							FieldNames: []string{"upc", "name", "price"},
						},
					},
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://product.service",
						},
						Subscription: SubscriptionConfiguration{
							URL: "ws://product.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") @key(fields: \"name\"){upc: String! name: String! price: Int!}",
						},
					}),
					Factory: federationFactory,
				},
				{
					RootNodes: []plan.TypeField{
						{
							TypeName:   "User",
							FieldNames: []string{"reviews"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"reviews"},
						},
					},
					ChildNodes: []plan.TypeField{
						{
							TypeName:   "Review",
							FieldNames: []string{"body", "author", "product"},
						},
						{
							TypeName:   "User",
							FieldNames: []string{"id", "username"},
						},
						{
							TypeName:   "Product",
							FieldNames: []string{"upc"},
						},
					},
					Factory: federationFactory,
					Custom: ConfigJson(Configuration{
						Fetch: FetchConfiguration{
							URL: "http://review.service",
						},
						Federation: FederationConfiguration{
							Enabled:    true,
							ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") @key(fields: \"name\") { upc: String! @external name: String! reviews: [Review] }",
						},
					}),
				},
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
					TypeName:       "User",
					FieldName:      "reviews",
					RequiresFields: []string{"id"},
				},
				{
					TypeName:       "Product",
					FieldName:      "name",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product",
					FieldName:      "price",
					RequiresFields: []string{"upc"},
				},
				{
					TypeName:       "Product",
					FieldName:      "reviews",
					RequiresFields: []string{"upc"},
				},
			},
		}))
}

var errSubscriptionClientFail = errors.New("subscription client fail error")

type FailingSubscriptionClient struct{}

func (f FailingSubscriptionClient) Subscribe(ctx context.Context, options GraphQLSubscriptionOptions, next chan<- []byte) error {
	return errSubscriptionClientFail
}

func TestSubscriptionSource_Start(t *testing.T) {
	chatServer := httptest.NewServer(chat.GraphQLEndpointHandler())
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
		subscriptionSource := SubscriptionSource{client: NewWebSocketGraphQLSubscriptionClient(&httpClient, ctx)}
		return subscriptionSource
	}

	t.Run("should return error when input is invalid", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": "", "header": null}`), nil)
		assert.Error(t, err)
	})

	t.Run("should return error when subscription client returns an error", func(t *testing.T) {
		source := SubscriptionSource{client: FailingSubscriptionClient{}}
		err := source.Start(context.Background(), []byte(`{"url": "", "body": {}, "header": null}`), nil)
		assert.Error(t, err)
		assert.Equal(t, resolve.ErrUnableToResolve, err)
	})

	t.Run("invalid json: should stop before sending to upstream", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: "#test") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.ErrorIs(t, err, resolve.ErrUnableToResolve)
	})

	t.Run("invalid syntax (roomNam)", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomNam: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		msg, ok := <-next
		assert.True(t, ok)
		assert.Equal(t, `{"errors":[{"message":"Unknown argument \"roomNam\" on field \"messageAdded\" of type \"Subscription\". Did you mean \"roomName\"?","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}},{"message":"Field \"messageAdded\" argument \"roomName\" of type \"String!\" is required but not provided.","locations":[{"line":1,"column":29}],"extensions":{"code":"GRAPHQL_VALIDATION_FAILED"}}]}`, string(msg))
		_, ok = <-next
		assert.False(t, ok)
	})

	t.Run("should close connection on stop message", func(t *testing.T) {
		next := make(chan []byte)
		subscriptionLifecycle, cancelSubscription := context.WithCancel(context.Background())
		resolverLifecycle, cancelResolver := context.WithCancel(context.Background())
		defer cancelResolver()

		source := newSubscriptionSource(resolverLifecycle)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(subscriptionLifecycle, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
		cancelSubscription()
		_, ok := <-next
		assert.False(t, ok)
	})

	t.Run("should successfully subscribe with chat example", func(t *testing.T) {
		next := make(chan []byte)
		ctx := context.Background()
		defer ctx.Done()

		source := newSubscriptionSource(ctx)
		chatSubscriptionOptions := chatServerSubscriptionOptions(t, `{"variables": {}, "extensions": {}, "operationName": "LiveMessages", "query": "subscription LiveMessages { messageAdded(roomName: \"#test\") { text createdBy } }"}`)
		err := source.Start(ctx, chatSubscriptionOptions, next)
		require.NoError(t, err)

		username := "myuser"
		message := "hello world!"
		go sendChatMessage(t, username, message)

		nextBytes := <-next
		assert.Equal(t, `{"data":{"messageAdded":{"text":"hello world!","createdBy":"myuser"}}}`, string(nextBytes))
	})
}

type _fakeDataSource struct {
	data              []byte
	artificialLatency time.Duration
}

func (f *_fakeDataSource) Load(ctx context.Context, input []byte, w io.Writer) (err error) {
	if f.artificialLatency != 0 {
		time.Sleep(f.artificialLatency)
	}
	_, err = w.Write(f.data)
	return
}

func FakeDataSource(data string) *_fakeDataSource {
	return &_fakeDataSource{
		data: []byte(data),
	}
}

func BenchmarkFederationBatching(b *testing.B) {

	userService := FakeDataSource(`{"data":{"me": {"id": "1234","username": "Me","__typename": "User"}}}`)
	reviewsService := FakeDataSource(`{"data":{"_entities":[{"reviews": [{"body": "A highly effective form of birth control.","product": {"upc": "top-1","__typename": "Product"}},{"body": "Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product": {"upc": "top-2","__typename": "Product"}}]}]}}`)
	productsService := FakeDataSource(`{"data":{"_entities":[{"name": "Trilby"},{"name": "Fedora"}]}}`)

	reviewBatchFactory := NewBatchFactory()
	productBatchFactory := NewBatchFactory()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resolver := resolve.New(ctx, resolve.NewFetcher(true), true)

	preparedPlan := &resolve.GraphQLResponse{
		Data: &resolve.Object{
			Fetch: &resolve.SingleFetch{
				BufferId: 0,
				InputTemplate: resolve.InputTemplate{
					Segments: []resolve.TemplateSegment{
						{
							Data:        []byte(`{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`),
							SegmentType: resolve.StaticSegmentType,
						},
					},
				},
				DataSource: userService,
				ProcessResponseConfig: resolve.ProcessResponseConfig{
					ExtractGraphqlResponse: true,
				},
			},
			Fields: []*resolve.Field{
				{
					HasBuffer: true,
					BufferID:  0,
					Name:      []byte("me"),
					Value: &resolve.Object{
						Fetch: &resolve.BatchFetch{
							Fetch: &resolve.SingleFetch{
								BufferId: 1,
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											Data:        []byte(`{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"`),
											SegmentType: resolve.StaticSegmentType,
										},
										{
											SegmentType:                  resolve.VariableSegmentType,
											VariableSource:               resolve.VariableSourceObject,
											VariableSourcePath:           []string{"id"},
											VariableValueType:            jsonparser.Number,
											RenderVariableAsGraphQLValue: true,
										},
										{
											Data:        []byte(`","__typename":"User"}]}}}`),
											SegmentType: resolve.StaticSegmentType,
										},
									},
								},
								DataSource: reviewsService,
								ProcessResponseConfig: resolve.ProcessResponseConfig{
									ExtractGraphqlResponse:    true,
									ExtractFederationEntities: true,
								},
							},
							BatchFactory: reviewBatchFactory,
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

								HasBuffer: true,
								BufferID:  1,
								Name:      []byte("reviews"),
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
												Name: []byte("product"),
												Value: &resolve.Object{
													Path: []string{"product"},
													Fetch: &resolve.BatchFetch{
														Fetch: &resolve.SingleFetch{
															BufferId:   2,
															DataSource: productsService,
															InputTemplate: resolve.InputTemplate{
																Segments: []resolve.TemplateSegment{
																	{
																		Data:        []byte(`{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																	{
																		SegmentType:                  resolve.VariableSegmentType,
																		VariableSource:               resolve.VariableSourceObject,
																		VariableSourcePath:           []string{"upc"},
																		VariableValueType:            jsonparser.String,
																		RenderVariableAsGraphQLValue: true,
																	},
																	{
																		Data:        []byte(`,"__typename":"Product"}]}}}`),
																		SegmentType: resolve.StaticSegmentType,
																	},
																},
															},
															ProcessResponseConfig: resolve.ProcessResponseConfig{
																ExtractGraphqlResponse:    true,
																ExtractFederationEntities: true,
															},
														},
														BatchFactory: productBatchFactory,
													},
													Fields: []*resolve.Field{
														{
															Name: []byte("upc"),
															Value: &resolve.String{
																Path: []string{"upc"},
															},
														},
														{
															HasBuffer: true,
															BufferID:  2,
															Name:      []byte("name"),
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
		},
	}

	var err error
	expected := []byte(`{"data":{"me":{"id":"1234","username":"Me","reviews":[{"body":"A highly effective form of birth control.","product":{"upc":"top-1","name":"Trilby"}},{"body":"Fedoras are one of the most fashionable hats around and can look great with a variety of outfits.","product":{"upc":"top-2","name":"Fedora"}}]}}}`)

	pool := sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 1024))
		},
	}

	ctxPool := sync.Pool{
		New: func() interface{} {
			return resolve.NewContext(context.Background())
		},
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(expected)))
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// _ = resolver.ResolveGraphQLResponse(ctx, plan, nil, ioutil.Discard)
			ctx := ctxPool.Get().(*resolve.Context)
			buf := pool.Get().(*bytes.Buffer)
			err = resolver.ResolveGraphQLResponse(ctx, preparedPlan, nil, buf)
			if err != nil {
				b.Fatal(err)
			}
			if !bytes.Equal(expected, buf.Bytes()) {
				b.Fatalf("want:\n%s\ngot:\n%s\n", string(expected), buf.String())
			}

			buf.Reset()
			pool.Put(buf)

			ctx.Free()
			ctxPool.Put(ctx)
		}
	})
}

const starWarsSchema = `
union SearchResult = Human | Droid | Starship

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
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

type Startship {
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
  title: String!
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

schema {
    query: Query
    mutation: Mutation
    subscription: Subscription
}

type Query {
    hero: Character
    droid(id: ID!): Droid
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

type Startship {
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
