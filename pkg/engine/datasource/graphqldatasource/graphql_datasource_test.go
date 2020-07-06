package graphqldatasource

import (
	"context"
	"encoding/json"
	"fmt"
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
					InputTemplate: resolve.InputTemplate{
						Segments: []resolve.TemplateSegment{
							{
								SegmentType: resolve.StaticSegmentType,
								Data:        []byte(`{"method":"POST","url":"https://swapi.com/graphql","body":{"query":"query($id: ID!){droid(id: $id){name aliased: name friends {name} primaryFunction} hero {name} stringList nestedStringList}","variables":{"id":"`),
							},
							{
								SegmentType:        resolve.VariableSegmentType,
								VariableSource:     resolve.VariableSourceContext,
								VariableSourcePath: []string{"id"},
							},
							{
								SegmentType: resolve.StaticSegmentType,
								Data:        []byte(`"}}}`),
							},
						},
					},
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
						InputTemplate: resolve.InputTemplate{
							Segments: []resolve.TemplateSegment{
								{
									SegmentType: resolve.StaticSegmentType,
									Data:        []byte(`{"method":"POST","url":"https://service.one","body":{"query":"mutation($name: String!){addFriend(name: $name){id name}}","variables":{"name":"`),
								},
								{
									SegmentType:        resolve.VariableSegmentType,
									VariableSource:     resolve.VariableSourceContext,
									VariableSourcePath: []string{"name"},
								},
								{
									SegmentType: resolve.StaticSegmentType,
									Data:        []byte(`"}}}`),
								},
							},
						},
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
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`{"method":"POST","url":"https://service.one","body":{"query":"query($firstArg: String, $thirdArg: Int){serviceOne(serviceOneArg: $firstArg){fieldOne} anotherServiceOne(anotherServiceOneArg: $thirdArg){fieldOne} reusingServiceOne(reusingServiceOneArg: $firstArg){fieldOne}}","variables":{"thirdArg":`),
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableSource:     resolve.VariableSourceContext,
											VariableSourcePath: []string{"thirdArg"},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`,"firstArg":"`),
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableSource:     resolve.VariableSourceContext,
											VariableSourcePath: []string{"firstArg"},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`"}}}`),
										},
									},
								},
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
								InputTemplate: resolve.InputTemplate{
									Segments: []resolve.TemplateSegment{
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`{"method":"POST","url":"https://service.two","body":{"query":"query($secondArg: Boolean, $fourthArg: Float){serviceTwo(serviceTwoArg: $secondArg){fieldTwo serviceOneField} secondServiceTwo(secondServiceTwoArg: $fourthArg){fieldTwo serviceOneField}}","variables":{"fourthArg":`),
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableSource:     resolve.VariableSourceContext,
											VariableSourcePath: []string{"fourthArg"},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`,"secondArg":`),
										},
										{
											SegmentType:        resolve.VariableSegmentType,
											VariableSource:     resolve.VariableSourceContext,
											VariableSourcePath: []string{"secondArg"},
										},
										{
											SegmentType: resolve.StaticSegmentType,
											Data:        []byte(`}}}`),
										},
									},
								},
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
											InputTemplate: resolve.InputTemplate{
												Segments: []resolve.TemplateSegment{
													{
														SegmentType: resolve.StaticSegmentType,
														Data: []byte(`{"method":"POST","url":"https://service.one","body":{"query":"query($a: String){serviceOne(serviceOneArg: $a){fieldOne}}","variables":{"a":"`),
													},
													{
														SegmentType: resolve.VariableSegmentType,
														VariableSource: resolve.VariableSourceObject,
														VariableSourcePath: []string{"serviceOneField"},
													},
													{
														SegmentType: resolve.StaticSegmentType,
														Data: []byte(`"}}}`),
													},
												},
											},
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
