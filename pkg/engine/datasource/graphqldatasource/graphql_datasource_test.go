package graphqldatasource

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/jensneuse/graphql-go-tools/pkg/engine/datasourcetesting"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/resolve"
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
		}
	`, "MyQuery", &plan.SynchronousResponsePlan{
		Response: resolve.GraphQLResponse{
			Data: &resolve.Object{
				Fetch: &resolve.SingleFetch{
					DataSource: &Source{
						Client: http.Client{
							Timeout: time.Second * 10,
						},
					},
					BufferId: 0,
					Input:    []byte(`{"url":"https://swapi.com/graphql","body":{"query":"query($id: ID!){droid(id: $id){name friends {name} primaryFunction} hero {name}}","variables":{"id":$$0$$}}}`),
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
									Path: []string{"droid"},
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
														Path: []string{"name"},
													},
												},
												{
													Name: []byte("friends"),
													Value: &resolve.Array{
														Path: []string{"friends"},
														Item: &resolve.Object{
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
						BufferID:   0,
						HasBuffer:  true,
						Fields: []resolve.Field{
							{
								Name: []byte("hero"),
								Value: &resolve.Object{
									Path: []string{"hero"},
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
				},
			},
		},
	}, plan.Configuration{
		DataSourceConfigurations: []plan.DataSourceConfiguration{
			{
				TypeName:   "Query",
				FieldNames: []string{"droid", "hero"},
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
											Name:       []byte("id"),
											Source:     FieldArgument,
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
	}))
	// TODO nesting with object arguments,
	// TODO test parallel fetch execution
	// TODO handle scalar lists (Path?)
	t.Run("nested graphql engines", RunTest(`
		type Query {
			serviceOne(serviceOneArg: String): ServiceOneResponse
			anotherServiceOne(anotherServiceOneArg: Int): ServiceOneResponse
			reusingServiceOne(reusingServiceOneArg: String): ServiceOneResponse
			serviceTwo(serviceTwoArg: Boolean): ServiceTwoResponse
			secondServiceTwo(secondServiceTwoArg: Float): ServiceTwoResponse
		}
		type ServiceOneResponse {
			fieldOne: String
		}
		type ServiceTwoResponse {
			fieldTwo: String
		}
	`, `
		query NestedQuery ($firstArg: String, $secondArg: Boolean, $thirdArg: Int, $fourthArg: Float){
			serviceOne(serviceOneArg: $firstArg) {
				fieldOne
			}
			serviceTwo(serviceTwoArg: $secondArg){
				fieldTwo
			}
			anotherServiceOne(anotherServiceOneArg: $thirdArg){
				fieldOne
			}
			secondServiceTwo(secondServiceTwoArg: $fourthArg){
				fieldTwo
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
						Fetches: []resolve.Fetch{
							&resolve.SingleFetch{
								BufferId: 0,
								Input:    []byte(`{"url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":$$1$$,"firstArg":$$0$$}}}`),
								DataSource: &Source{
									Client: http.Client{
										Timeout: time.Second * 10,
									},
								},
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
								BufferId: 1,
								Input:    []byte(`{"url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo}}","variables":{"fourthArg":$$1$$,"secondArg":$$0$$}}}`),
								DataSource: &Source{
									Client: http.Client{
										Timeout: time.Second * 10,
									},
								},
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
										Path: []string{"serviceOne"},
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
										Path: []string{"serviceTwo"},
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("fieldTwo"),
														Value: &resolve.String{
															Path: []string{"fieldTwo"},
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
										Path: []string{"anotherServiceOne"},
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
										Path: []string{"secondServiceTwo"},
										FieldSets: []resolve.FieldSet{
											{
												Fields: []resolve.Field{
													{
														Name: []byte("fieldTwo"),
														Value: &resolve.String{
															Path: []string{"fieldTwo"},
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
										Path: []string{"reusingServiceOne"},
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
					FieldNames: []string{"serviceOne","anotherServiceOne","reusingServiceOne"},
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
												Name:   []byte("serviceOneArg"),
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "anotherServiceOne",
										Arguments: []Argument{
											{
												Name:   []byte("anotherServiceOneArg"),
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "reusingServiceOne",
										Arguments: []Argument{
											{
												Name:   []byte("reusingServiceOneArg"),
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
					FieldNames: []string{"serviceTwo","secondServiceTwo"},
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
												Name:   []byte("serviceTwoArg"),
												Source: FieldArgument,
											},
										},
									},
									{
										FieldName: "secondServiceTwo",
										Arguments: []Argument{
											{
												Name:   []byte("secondServiceTwoArg"),
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
}

func TestGraphQLDataSourceExecution(t *testing.T) {
	test := func(ctx func() context.Context, input func(server *httptest.Server) string, serverHandler func(t *testing.T) http.HandlerFunc, result func(t *testing.T, bufPair *resolve.BufPair, err error)) func(t *testing.T) {
		return func(t *testing.T) {
			server := httptest.NewServer(serverHandler(t))
			defer server.Close()
			source := &Source{}
			bufPair := &resolve.BufPair{
				Data:   &bytes.Buffer{},
				Errors: &bytes.Buffer{},
			}
			err := source.Load(ctx(), []byte(input(server)), bufPair)
			result(t, bufPair, err)
		}
	}

	t.Run("simple", test(func() context.Context {
		return context.Background()
	}, func(server *httptest.Server) string {
		return fmt.Sprintf(`{"url":"%s","body":{"query":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}}`, server.URL)
	}, func(t *testing.T) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			body, err := ioutil.ReadAll(request.Body)
			assert.NoError(t, err)
			assert.Equal(t, `{"query":"query($id: ID!){droid(id: $id){name}}","variables":{"id":1}}`, string(body))
			assert.Equal(t, request.Method, http.MethodPost)
			_, err = writer.Write([]byte(`{"data":{"droid":{"name":"r2d2"}}"}`))
			assert.NoError(t, err)
		}
	}, func(t *testing.T, bufPair *resolve.BufPair, err error) {
		assert.NoError(t, err)
		assert.Equal(t, `{"droid":{"name":"r2d2"}}`, bufPair.Data.String())
		assert.Equal(t, false, bufPair.HasErrors())
	}))
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
