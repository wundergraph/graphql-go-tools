package graphql_datasource

import (
	"net/http"
	"testing"

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
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","header":{"Authorization":["$$1$$"],"Invalid-Template":["{{ request.headers.Authorization }}"]},"body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":"$$0$$"}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path: []string{"id"},
						},
						&resolve.HeaderVariable{
							Path: []string{"Authorization"},
						},
					),
				},
				Fields: []*resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("droid"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("hero"),
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
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("stringList"),
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
						Input:      `{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"name"},
							},
						),
						DisallowSingleFlight: true,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("addFriend"),
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
						Input:      `{"method":"POST","url":"https://foo.service","body":{"query":"query($a: String, $b: String){foo(bar: $a){bar(bal: $b)}}","variables":{"b":"$$1$$","a":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
							&resolve.ContextVariable{
								Path: []string{"b"},
							},
						),
						DisallowSingleFlight: false,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("foo"),
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
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} alias: country(code: $b){name}}","variables":{"b":"$$1$$","a":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
							&resolve.ContextVariable{
								Path: []string{"b"},
							},
						),
						DisallowSingleFlight: false,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
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
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("alias"),
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
						Input:      `{"method":"POST","url":"https://countries.service","body":{"query":"query($a: ID!, $b: ID!){country(code: $a){name} countryAlias: country(code: $b){name}}","variables":{"b":"$$1$$","a":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"a"},
							},
							&resolve.ContextVariable{
								Path: []string{"b"},
							},
						),
						DisallowSingleFlight: false,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("country"),
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
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("countryAlias"),
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
								Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":"$$0$$"}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path: []string{"firstArg"},
									},
									&resolve.ContextVariable{
										Path: []string{"thirdArg"},
									},
								),
							},
							&resolve.SingleFetch{
								BufferId:   2,
								Input:      `{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo serviceOneField} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
								DataSource: &Source{},
								Variables: resolve.NewVariables(
									&resolve.ContextVariable{
										Path: []string{"secondArg"},
									},
									&resolve.ContextVariable{
										Path: []string{"fourthArg"},
									},
								),
							},
						},
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("serviceOne"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceOne"},

								Fetch: &resolve.SingleFetch{
									BufferId:   1,
									DataSource: &Source{},
									Input:      `{"method":"POST","url":"https://country.service","body":{"query":"{countries {name}}"}}`,
								},

								Fields: []*resolve.Field{
									{
										Name: []byte("fieldOne"),
										Value: &resolve.String{
											Path: []string{"fieldOne"},
										},
									},
									{
										Name:      []byte("countries"),
										HasBuffer: true,
										BufferID:  1,
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
							HasBuffer: true,
							BufferID:  2,
							Name:      []byte("serviceTwo"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceTwo"},
								Fetch: &resolve.SingleFetch{
									BufferId:   3,
									DataSource: &Source{},
									Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOneResponse: serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":"$$0$$"}}}`,
									Variables: resolve.NewVariables(
										&resolve.ObjectVariable{
											Path: []string{"serviceOneField"},
										},
									),
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
										HasBuffer: true,
										BufferID:  3,
										Name:      []byte("serviceOneResponse"),
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
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("anotherServiceOne"),
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
							BufferID:  2,
							HasBuffer: true,
							Name:      []byte("secondServiceTwo"),
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
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("reusingServiceOne"),
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
						Input:      `{"method":"POST","url":"https://graphql.service","body":{"query":"mutation($title: String!, $completed: Boolean!, $name: String!){addTask(input: [{title: $title,completed: $completed,user: {name: $name}}]){task {id title completed}}}","variables":{"name":"$$2$$","completed":$$1$$,"title":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"title"},
							},
							&resolve.ContextVariable{
								Path: []string{"completed"},
							},
							&resolve.ContextVariable{
								Path: []string{"name"},
							},
						),
						DisallowSingleFlight: true,
					},
					Fields: []*resolve.Field{
						{
							HasBuffer: true,
							BufferID:  0,
							Name:      []byte("addTask"),
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
						Input:      `{"method":"POST","url":"https://user.service","body":{"query":"mutation($id: String, $name: String){createUser(input: {user: {id: $id,username: $name}}){user {id username createdDate}}}","variables":{"name":"$$1$$","id":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"id"},
							},
							&resolve.ContextVariable{
								Path: []string{"name"},
							},
						),
						DisallowSingleFlight: true,
					},
					Fields: []*resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("createUser"),
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
						Input:      `{"method":"POST","url":"http://api.com","body":{"query":"mutation($name: String!, $personal: Boolean!){__typename namespaceCreate(input: {name: $name,personal: $personal}){__typename ... on NamespaceCreated {namespace {id name}} ... on Error {code message}}}","variables":{"personal":$$1$$,"name":"$$0$$"}}}`,
						DataSource: &Source{},
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"name"},
							},
							&resolve.ContextVariable{
								Path: []string{"personal"},
							},
						),
						DisallowSingleFlight: true,
					},
					Fields: []*resolve.Field{
						{
							Name: []byte("__typename"),
							Value: &resolve.String{
								Path:     []string{"__typename"},
								Nullable: false,
							},
						},
						{
							Name:      []byte("namespaceCreate"),
							HasBuffer: true,
							BufferID:  0,
							Value: &resolve.Object{
								Path: []string{"namespaceCreate"},
								Fields: []*resolve.Field{
									{
										Name: []byte("__typename"),
										Value: &resolve.String{
											Path:     []string{"__typename"},
											Nullable: false,
										},
									},
									{
										OnTypeName: []byte("NamespaceCreated"),
										Name:       []byte("namespace"),
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
										OnTypeName: []byte("Error"),
										Name:       []byte("code"),
										Value: &resolve.String{
											Path: []string{"code"},
										},
									},
									{
										OnTypeName: []byte("Error"),
										Name:       []byte("message"),
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
			DefaultFlushInterval: 500,
		}))

	t.Run("subscription", RunTest(testDefinition, `
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				ManagerID: []byte("graphql_websocket_subscription"),
				Input:     `{"url":"wss://swapi.com/graphql","body":{"query":"subscription{remainingJedis}"}}`,
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
				Factory: &Factory{},
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
		Response: resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				ManagerID: []byte("graphql_websocket_subscription"),
				Input:     `{"url":"wss://swapi.com/graphql","body":{"query":"subscription($a: String){foo(bar: $a)}","variables":{"a":"$$0$$"}}}`,
				Variables: resolve.NewVariables(
					&resolve.ContextVariable{
						Path: []string{"a"},
					},
				),
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
				Factory: &Factory{},
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
						BufferId:   0,
						Input:      `{"method":"POST","url":"http://user.service","body":{"query":"{me {id username}}"}}`,
						DataSource: &Source{},
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
										Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body author {id username} product {upc}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}},"extract_entities":true}`,
										Variables: resolve.NewVariables(
											&resolve.ObjectVariable{
												Path: []string{"id"},
											},
										),
										DataSource: &Source{},
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
															Fetch: &resolve.ParallelFetch{
																Fetches: []resolve.Fetch{
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId:   2,
																			Input:      `{"method":"POST","url":"http://product.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name price}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}},"extract_entities":true}`,
																			DataSource: &Source{},
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path: []string{"upc"},
																				},
																			),
																		},
																		BatchFactory: batchFactory,
																	},
																	&resolve.BatchFetch{
																		Fetch: &resolve.SingleFetch{
																			BufferId: 3,
																			Input:    `{"method":"POST","url":"http://review.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {reviews {body author {id username}}}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}},"extract_entities":true}`,
																			Variables: resolve.NewVariables(
																				&resolve.ObjectVariable{
																					Path: []string{"upc"},
																				},
																			),
																			DataSource: &Source{},
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
																},
																{
																	HasBuffer: true,
																	BufferID:  2,
																	Name:      []byte("price"),
																	Value: &resolve.Integer{
																		Path: []string{"price"},
																	},
																},
																{
																	HasBuffer: true,
																	BufferID:  3,
																	Name:      []byte("reviews"),
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
							ServiceSDL: "extend type Query {topProducts(first: Int = 5): [Product]} type Product @key(fields: \"upc\") {upc: String! name: String! price: Int!}",
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
							ServiceSDL: "type Review { body: String! author: User! @provides(fields: \"username\") product: Product! } extend type User @key(fields: \"id\") { id: ID! @external reviews: [Review] } extend type Product @key(fields: \"upc\") { upc: String! @external reviews: [Review] }",
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
