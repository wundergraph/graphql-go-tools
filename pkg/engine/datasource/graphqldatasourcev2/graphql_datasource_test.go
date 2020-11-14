package graphqldatasourcev2

import (
	"encoding/json"
	"testing"

	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetestingv2"
	plan "github.com/jensneuse/graphql-go-tools/pkg/engine/planv2"
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
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":"$$0$$"}}}`,
					Variables: resolve.NewVariables(
						&resolve.ContextVariable{
							Path: []string{"id"},
						},
					),
				},
				Fields: []resolve.Field{
					{
						HasBuffer: true,
						BufferID:  0,
						Name:      []byte("droid"),
						Value: &resolve.Object{
							Path:     []string{"droid"},
							Nullable: true,
							Fields: []resolve.Field{
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
											Fields: []resolve.Field{
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
							Fields: []resolve.Field{
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
				Factory:                    &Factory{},
				OverrideFieldPathFromAlias: true,
				Custom: ConfigJson(Configuration{
					URL: "https://swapi.com/graphql",
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
					Fields: []resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("addFriend"),
							Value: &resolve.Object{
								Fields: []resolve.Field{
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
					OverrideFieldPathFromAlias: true,
					Custom: ConfigJson(Configuration{
						URL: "https://service.one",
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
					Fields: []resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("foo"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"foo"},
								Fields: []resolve.Field{
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
					Factory:                    &Factory{},
					OverrideFieldPathFromAlias: true,
					Custom: ConfigJson(Configuration{
						URL: "https://foo.service",
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
		}
		type ServiceTwoResponse {
			fieldTwo: String
			serviceOneField: String
			serviceOneResponse: ServiceOneResponse
		}
	`, `
		query NestedQuery ($firstArg: String, $secondArg: Boolean, $thirdArg: Int, $fourthArg: Float){
			serviceOne(serviceOneArg: $firstArg) {
				fieldOne
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
						Fetches: []*resolve.SingleFetch{
							{
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
							{
								BufferId:   1,
								// 			 {"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}
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
					Fields: []resolve.Field{
						{
							BufferID:  0,
							HasBuffer: true,
							Name:      []byte("serviceOne"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceOne"},
								Fields: []resolve.Field{
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
							HasBuffer: true,
							BufferID:  1,
							Name:      []byte("serviceTwo"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"serviceTwo"},
								Fetch: &resolve.SingleFetch{
									BufferId:   2,
									DataSource: &Source{},
									Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":"$$0$$"}}}`,
									Variables: resolve.NewVariables(
										&resolve.ObjectVariable{
											Path: []string{"serviceOneField"},
										},
									),
								},
								Fields: []resolve.Field{
									{
										Name: []byte("fieldTwo"),
										Value: &resolve.String{
											Nullable: true,
											Path:     []string{"fieldTwo"},
										},
									},
									{
										HasBuffer: true,
										BufferID:  2,
										Name:      []byte("serviceOneResponse"),
										Value: &resolve.Object{
											Nullable: true,
											Path:     []string{"serviceOne"},
											Fields: []resolve.Field{
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
								Fields: []resolve.Field{
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
							BufferID:  1,
							HasBuffer: true,
							Name:      []byte("secondServiceTwo"),
							Value: &resolve.Object{
								Nullable: true,
								Path:     []string{"secondServiceTwo"},
								Fields: []resolve.Field{
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
								Fields: []resolve.Field{
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
						URL: "https://service.one",
					}),
					Factory:                    nestedGraphQLEngineFactory,
					OverrideFieldPathFromAlias: true,
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
						URL: "https://service.two",
					}),
					Factory:                    nestedGraphQLEngineFactory,
					OverrideFieldPathFromAlias: true,
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
}

func ConfigJson(config Configuration) json.RawMessage {
	out, _ := json.Marshal(config)
	return out
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
