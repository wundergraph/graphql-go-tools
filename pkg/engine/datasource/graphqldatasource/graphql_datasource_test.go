package graphqldatasource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
)

func TestGraphQLDataSourcePlanning(t *testing.T) {
	t.Run("simple named Query", RunTest(testDefinition, `
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
		Response: resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: DefaultSource(),
					BufferId:   0,
					Input:      `{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":"$$0$$"}}}`,
					Variables: resolve.NewVariables(&resolve.ContextVariable{
						Path: []string{"id"},
					}),
				},
				FieldSets: []resolve.FieldSet{
					{
						HasBuffer: true,
						BufferID:  0,
						Fields: []resolve.Field{
							{
								Name: []byte("droid"),
								Value: &resolve.Object{
									Path:     []string{"droid"},
									Nullable: true,
									FieldSets: []resolve.FieldSet{
										{
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
															FieldSets: []resolve.FieldSet{
																{
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
								},
							},
						},
					},
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []resolve.Field{
							{
								Name: []byte("hero"),
								Value: &resolve.Object{
									Path:     []string{"hero"},
									Nullable: true,
									FieldSets: []resolve.FieldSet{
										{
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
							},
						},
					},
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []resolve.Field{
							{
								Name: []byte("stringList"),
								Value: &resolve.Array{
									Nullable: true,
									Item: &resolve.String{
										Nullable: true,
									},
								},
							},
						},
					},
					{
						BufferID:  0,
						HasBuffer: true,
						Fields: []resolve.Field{
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
			},
		},
	}, plan.Configuration{
		DataSourceConfigurations: []plan.DataSourceConfiguration{
			{
				TypeName:   "Query",
				FieldNames: []string{"droid", "hero", "stringList", "nestedStringList"},
				Attributes: []plan.DataSourceAttribute{
					{
						Key:   "url",
						Value: []byte("https://swapi.com/graphql"),
					},
					{
						Key: "arguments",
						Value: ArgumentsConfigJSON(ArgumentsConfig{
							Fields: []FieldConfig{
								{
									FieldName: "droid",
									Arguments: []Argument{
										{
											Name:   "id",
											Source: FieldArgument,
										},
									},
								},
							},
						}),
					},
				},
				DataSourcePlanner: &Planner{},
			},
		},
		FieldMappings: []plan.FieldMapping{
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
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    `{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":"$$0$$"}}}`,
						DataSource: DefaultSource(),
						Variables: resolve.NewVariables(
							&resolve.ContextVariable{
								Path: []string{"name"},
							},
						),
						DisallowSingleFlight: true,
					},
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("addFriend"),
									Value: &resolve.Object{
										FieldSets: []resolve.FieldSet{
											{
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
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addFriend"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://service.one"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "addFriend",
										Arguments: []Argument{
											{
												Name:   "name",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:              "Mutation",
					FieldName:             "addFriend",
					DisableDefaultMapping: true,
				},
			},
		},
	))
	nestedResolverPlanner := &Planner{}
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
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    `{"method":"POST","url":"https://foo.service","body":{"query":"query($a: String, $b: String){foo(bar: $a){bar(bal: $b)}}","variables":{"b":"$$1$$","a":"$$0$$"}}}`,
						DataSource: DefaultSource(),
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
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("foo"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"foo"},
										FieldSets: []resolve.FieldSet{
											{
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
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"foo"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://foo.service"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "foo",
										Arguments: []Argument{
											{
												Name:   "bar",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner:        nestedResolverPlanner,
					UpstreamUniqueIdentifier: "foo",
				},
				{
					TypeName:   "Baz",
					FieldNames: []string{"bar"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://foo.service"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "bar",
										Arguments: []Argument{
											{
												Name:   "bal",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner:        nestedResolverPlanner,
					UpstreamUniqueIdentifier: "foo",
				},
			},
		},
	))
	// TODO: add validation to check if all required field dependencies are met
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
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.ParallelFetch{
						Fetches: []*resolve.SingleFetch{
							{
								BufferId: 0,
								Input:    `{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":"$$0$$"}}}`,
								DataSource: DefaultSource(),
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
								BufferId: 1,
								Input:    `{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo serviceOneField} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`,
								DataSource: DefaultSource(),
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
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("serviceOne"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"serviceOne"},
										FieldSets: []resolve.FieldSet{
											{
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
						{
							BufferID:  1,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("serviceTwo"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"serviceTwo"},
										Fetch: &resolve.SingleFetch{
											BufferId:   2,
											DataSource: DefaultSource(),
											Input:      `{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":"$$0$$"}}}`,
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path: []string{"serviceOneField"},
												},
											),
										},
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("fieldTwo"),
														Value: &resolve.String{
															Nullable: true,
															Path:     []string{"fieldTwo"},
														},
													},
												},
											},
											{
												BufferID:  2,
												HasBuffer: true,
												Fields: []resolve.Field{
													{
														Name: []byte("serviceOneResponse"),
														Value: &resolve.Object{
															Nullable: true,
															Path:     []string{"serviceOne"},
															FieldSets: []resolve.FieldSet{
																{
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
										},
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("anotherServiceOne"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"anotherServiceOne"},
										FieldSets: []resolve.FieldSet{
											{
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
						{
							BufferID:  1,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("secondServiceTwo"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"secondServiceTwo"},
										FieldSets: []resolve.FieldSet{
											{
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
									},
								},
							},
						},
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("reusingServiceOne"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"reusingServiceOne"},
										FieldSets: []resolve.FieldSet{
											{
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
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"serviceOne", "anotherServiceOne", "reusingServiceOne"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://service.one"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "serviceOne",
										Arguments: []Argument{
											{
												Name:   "serviceOneArg",
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "anotherServiceOne",
										Arguments: []Argument{
											{
												Name:   "anotherServiceOneArg",
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "reusingServiceOne",
										Arguments: []Argument{
											{
												Name:   "reusingServiceOneArg",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner: &Planner{},
				},
				{
					TypeName:   "Query",
					FieldNames: []string{"serviceTwo", "secondServiceTwo"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://service.two"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "serviceTwo",
										Arguments: []Argument{
											{
												Name:   "serviceTwoArg",
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "secondServiceTwo",
										Arguments: []Argument{
											{
												Name:   "secondServiceTwoArg",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner: &Planner{},
				},
				{
					TypeName:   "ServiceTwoResponse",
					FieldNames: []string{"serviceOneResponse"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://service.one"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "serviceOneResponse",
										Arguments: []Argument{
											{
												Name:       "serviceOneArg",
												Source:     ObjectField,
												SourcePath: []string{"serviceOneField"},
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{
				{
					TypeName:  "ServiceTwoResponse",
					FieldName: "serviceOneResponse",
					Path:      []string{"serviceOne"},
				},
			},
			FieldDependencies: []plan.FieldDependency{
				{
					TypeName:       "ServiceTwoResponse",
					FieldName:      "serviceOneResponse",
					RequiresFields: []string{"serviceOneField"},
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
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    `{"method":"POST","url":"https://graphql.service","body":{"query":"mutation($title: String!, $completed: Boolean!, $name: String!){addTask(input: [{title: $title,completed: $completed,user: {name: $name}}]){task {id title completed}}}","variables":{"name":"$$2$$","completed":$$1$$,"title":"$$0$$"}}}`,
						DataSource: DefaultSource(),
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
					FieldSets: []resolve.FieldSet{
						{
							HasBuffer: true,
							BufferID:  0,
							Fields: []resolve.Field{
								{
									Name: []byte("addTask"),
									Value: &resolve.Object{
										Path:     []string{"addTask"},
										Nullable: true,
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("task"),
														Value: &resolve.Array{
															Nullable: true,
															Path:     []string{"task"},
															Item: &resolve.Object{
																Nullable: true,
																FieldSets: []resolve.FieldSet{
																	{
																		Fields: []resolve.Field{
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
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Mutation",
					FieldNames: []string{"addTask"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://graphql.service"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "addTask",
										Arguments: []Argument{
											{
												Name:   "input",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner:        &Planner{},
					UpstreamUniqueIdentifier: "graphql.service",
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
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId: 0,
						Input:    `{"method":"POST","url":"https://user.service","body":{"query":"mutation($id: String, $name: String){createUser(input: {user: {id: $id,username: $name}}){user {id username createdDate}}}","variables":{"name":"$$1$$","id":"$$0$$"}}}`,
						DataSource: DefaultSource(),
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
					FieldSets: []resolve.FieldSet{
						{
							BufferID:  0,
							HasBuffer: true,
							Fields: []resolve.Field{
								{
									Name: []byte("createUser"),
									Value: &resolve.Object{
										Nullable: true,
										Path:     []string{"createUser"},
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("user"),
														Value: &resolve.Object{
															Path:     []string{"user"},
															Nullable: true,
															FieldSets: []resolve.FieldSet{
																{
																	Fields: []resolve.Field{
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
							},
						},
					},
				},
			},
		},
		plan.Configuration{
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Mutation",
					FieldNames: []string{"createUser"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("https://user.service"),
						},
						{
							Key: "arguments",
							Value: ArgumentsConfigJSON(ArgumentsConfig{
								Fields: []FieldConfig{
									{
										FieldName: "createUser",
										Arguments: []Argument{
											{
												Name:   "input",
												Source: FieldArgument,
											},
										},
									},
								},
							}),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
		},
	))
	t.Run("subscription", RunTest(testDefinition, `
		subscription RemainingJedis {
			remainingJedis
		}
	`, "RemainingJedis", &plan.SubscriptionResponsePlan{
		Response: resolve.GraphQLSubscription{
			Trigger: resolve.GraphQLSubscriptionTrigger{
				ManagerID: []byte("graphql_websocket_subscription"),
				Input:     `{"scheme":"wss","host":"swapi.com","path":"/graphql","body":{"query":"subscription{remainingJedis}"}}`,
			},
			Response: &resolve.GraphQLResponse{
				Data: &resolve.Object{
					FieldSets: []resolve.FieldSet{
						{
							Fields: []resolve.Field{
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
			},
		},
	}, plan.Configuration{
		DataSourceConfigurations: []plan.DataSourceConfiguration{
			{
				TypeName:   "Subscription",
				FieldNames: []string{"remainingJedis"},
				Attributes: []plan.DataSourceAttribute{
					{
						Key:   "url",
						Value: []byte("https://swapi.com/graphql"),
					},
				},
				DataSourcePlanner: &Planner{},
			},
		},
	}))
	t.Run("simple", RunTest(federationTestSchema,
		`query MyReviews {
					  me {
						id
						username
						reviews {
						  body
						  product {
							name
						  }
						}
					  }
					}`,
		"MyReviews",
		&plan.SynchronousResponsePlan{
			Response: resolve.GraphQLResponse{
				Data: &resolve.Object{
					Fetch: &resolve.SingleFetch{
						BufferId:   0,
						Input:      `{"method":"POST","url":"http://localhost:4001","body":{"query":"{me {id username}}"}}`,
						DataSource: DefaultSource(),
					},
					FieldSets: []resolve.FieldSet{
						{
							HasBuffer: true,
							BufferID:  0,
							Fields: []resolve.Field{
								{
									Name: []byte("me"),
									Value: &resolve.Object{
										Fetch: &resolve.SingleFetch{
											BufferId:   1,
											Input:      `{"method":"POST","url":"http://localhost:4002","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on User {reviews {body product {upc __typename}}}}}","variables":{"representations":[{"id":"$$0$$","__typename":"User"}]}},"extract_entities":true}`,
											Variables: resolve.NewVariables(
												&resolve.ObjectVariable{
													Path: []string{"id"},
												},
											),
											DataSource: DefaultSource(),
										},
										Path:     []string{"me"},
										Nullable: true,
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
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
											{
												HasBuffer: true,
												BufferID: 1,
												Fields: []resolve.Field{
													{
														Name: []byte("reviews"),
														Value: &resolve.Array{
															Path: []string{"reviews"},
															Nullable: true,
															Item: &resolve.Object{
																Nullable: true,
																FieldSets: []resolve.FieldSet{
																	{
																		Fields: []resolve.Field{
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
																					Fetch: &resolve.SingleFetch{
																						BufferId:   2,
																						Input:      `{"method":"POST","url":"http://localhost:4003","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){... on Product {name}}}","variables":{"representations":[{"upc":"$$0$$","__typename":"Product"}]}},"extract_entities":true}`,
																						DataSource: DefaultSource(),
																						Variables: resolve.NewVariables(
																							&resolve.ObjectVariable{
																								Path: []string{"upc"},
																							},
																						),
																					},
																					FieldSets: []resolve.FieldSet{
																						{
																							HasBuffer: true,
																							BufferID: 2,
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
																			},
																		},
																	},
																},
															},
														},
													},
												},
											},
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
			DataSourceConfigurations: []plan.DataSourceConfiguration{
				{
					TypeName:   "Query",
					FieldNames: []string{"me"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("http://localhost:4001"),
						},
						{
							Key: "federation_service_sdl",
							Value: []byte(`extend type Query {me: User} type User @key(fields: "id"){ id: ID! username: String!}`),
						},
					},
					DataSourcePlanner: &Planner{},
				},
				{
					TypeName:   "Product",
					FieldNames: []string{"name","price"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("http://localhost:4003"),
						},
						{
							Key:   "federation_service_sdl",
							Value: []byte(`extend type Query {topProducts(first: Int = 5): [Product]}type Product @key(fields: "upc") {upc: String!name: String! price: Int!}`),
						},
					},
					DataSourcePlanner: &Planner{},
				},
				{
					TypeName:   "User",
					FieldNames: []string{"reviews"},
					Attributes: []plan.DataSourceAttribute{
						{
							Key:   "url",
							Value: []byte("http://localhost:4002"),
						},
						{
							Key:   "federation_service_sdl",
							Value: []byte(`type Review { body: String! author: User! @provides(fields: "username") product: Product! } extend type User @key(fields: "id") { id: ID! @external reviews: [Review] } extend type Product @key(fields: "upc") { upc: String! @external reviews: [Review] } `),
						},
					},
					DataSourcePlanner: &Planner{},
				},
			},
			FieldMappings: []plan.FieldMapping{},
		},
	))
}

type fakeHttpClient struct {
	data []byte
}

func (f *fakeHttpClient) Do(ctx context.Context, requestInput []byte, out io.Writer) (err error) {
	_,err = out.Write(f.data)
	return
}

func TestGraphQLDataSourceEntitiesExtraction(t *testing.T){
	t.Run("extraction false", func(t *testing.T) {
		source := &Source{
			client:  &fakeHttpClient{
				data: []byte(`{"data":{"foo":"bar"}}`),
			},
		}
		bufPair := resolve.NewBufPair()
		err := source.Load(context.Background(),nil,bufPair)
		assert.NoError(t,err)
		assert.Equal(t,`{"foo":"bar"}`,bufPair.Data.String())
	})
	t.Run("extraction true", func(t *testing.T) {
		source := &Source{
			client:  &fakeHttpClient{
				data: []byte(`{"data":{"_entities":[{"foo":"bar"}]}}`),
			},
		}
		bufPair := resolve.NewBufPair()
		err := source.Load(context.Background(),[]byte(`{"extract_entities":true}`),bufPair)
		assert.NoError(t,err)
		assert.Equal(t,`{"foo":"bar"}`,bufPair.Data.String())
	})
}

func TestGraphQLDataSourceExecution(t *testing.T) {
	test := func(ctx func() context.Context, input func(server *httptest.Server) string, serverHandler func(t *testing.T) http.HandlerFunc, result func(t *testing.T, bufPair *resolve.BufPair, err error)) func(t *testing.T) {
		return func(t *testing.T) {
			server := httptest.NewServer(serverHandler(t))
			defer server.Close()
			source := DefaultSource()
			bufPair := &resolve.BufPair{
				Data:   fastbuffer.New(),
				Errors: fastbuffer.New(),
			}
			err := source.Load(ctx(), []byte(input(server)), bufPair)
			result(t, bufPair, err)
		}
	}

	t.Run("simple", test(func() context.Context {
		return context.Background()
	}, func(server *httptest.Server) string {
		return fmt.Sprintf(`{"method":"POST","url":"%s","body":{"query":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}}`, server.URL)
	}, func(t *testing.T) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			body, err := ioutil.ReadAll(request.Body)
			assert.NoError(t, err)
			assert.Equal(t, `{"query":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}`, string(body))
			assert.Equal(t, http.MethodPost, request.Method)
			_, err = writer.Write([]byte(`{"data":{"droid":{"name":"r2d2"}}"}`))
			assert.NoError(t, err)
		}
	}, func(t *testing.T, bufPair *resolve.BufPair, err error) {
		assert.NoError(t, err)
		assert.Equal(t, `{"droid":{"name":"r2d2"}}`, bufPair.Data.String())
		assert.Equal(t, false, bufPair.HasErrors())
	}))
}

func TestParseArguments(t *testing.T) {
	input := `{"fields":[{"field_name":"continents","arguments":[{"name":"filter","source":"field_argument","source_path":["filter"]}]},{"field_name":"continent","arguments":[{"name":"code","source":"field_argument","source_path":["code"]}]},{"field_name":"countries","arguments":[{"name":"filter","source":"field_argument","source_path":["filter"]}]},{"field_name":"country","arguments":[{"name":"code","source":"field_argument","source_path":["code"]}]},{"field_name":"languages","arguments":[{"name":"filter","source":"field_argument","source_path":["filter"]}]},{"field_name":"language","arguments":[{"name":"code","source":"field_argument","source_path":["code"]}]}]}`
	var args ArgumentsConfig
	err := json.Unmarshal([]byte(input), &args)
	assert.NoError(t, err)
}

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
