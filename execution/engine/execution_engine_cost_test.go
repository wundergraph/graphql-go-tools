package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Cost(t *testing.T) {

	t.Run("common on star wars scheme", func(t *testing.T) {
		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"hero", "droid"}},
			{TypeName: "Human", FieldNames: []string{"name", "height", "friends"}},
			{TypeName: "Droid", FieldNames: []string{"name", "primaryFunction", "friends"}},
		}
		childNodes := []plan.TypeField{
			{TypeName: "Character", FieldNames: []string{"name", "friends"}},
		}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(
				t,
				nil,
				string(graphql.StarwarsSchema(t).RawSchema()),
			),
		})

		t.Run("droid with weighted plain fields", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
								droid(id: "R2D2") {
									name
									primaryFunction
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
								},
							}},
						customConfig,
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName: "Query", FieldName: "droid",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "id",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse:      `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
				expectedEstimatedCost: 18, // Query.droid (1) + droid.name (17)
			},
			computeCosts(),
		))

		t.Run("droid with weighted plain fields and an argument", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
								droid(id: "R2D2") {
									name
									primaryFunction
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Query", FieldName: "droid"}: {
										ArgumentWeights: map[string]int{"id": 3},
										HasWeight:       false,
									},
									{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
								},
							}},
						customConfig,
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName: "Query", FieldName: "droid",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "id",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse:      `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
				expectedEstimatedCost: 21, // Query.droid (1) + Query.droid.id (3) + droid.name (17)
			},
			computeCosts(),
		))

		t.Run("negative weights - cost is never negative", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
								droid(id: "R2D2") {
									name
									primaryFunction
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Query", FieldName: "droid"}: {
										HasWeight:       true,
										Weight:          -10,                      // Negative field weight
										ArgumentWeights: map[string]int{"id": -5}, // Negative argument weight
									},
									{TypeName: "Droid", FieldName: "name"}:            {HasWeight: true, Weight: -3},
									{TypeName: "Droid", FieldName: "primaryFunction"}: {HasWeight: true, Weight: -2},
								},
								Types: map[string]int{
									"Droid": -1, // Negative type weight
								},
							}},
						customConfig,
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName: "Query", FieldName: "droid",
						Arguments: []plan.ArgumentConfiguration{
							{
								Name:         "id",
								SourceType:   plan.FieldArgumentSource,
								RenderConfig: plan.RenderArgumentAsGraphQLValue,
							},
						},
					},
				},
				expectedResponse: `{"data":{"droid":{"name":"R2D2","primaryFunction":"no"}}}`,
				// All weights are negative.
				// But cost should be floored to 0 (never negative)
				expectedEstimatedCost: 0,
			},
			computeCosts(),
		))

		t.Run("hero field has weight (returns interface) and with concrete fragment", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero { 
									name 
									... on Human { height }
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke Skywalker","height":"12"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
							Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
								{TypeName: "Query", FieldName: "hero"}:   {HasWeight: true, Weight: 2},
								{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 3},
								{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 7},
								{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 17},
							},
							Types: map[string]int{
								"Human": 13,
							},
						}},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"name":"Luke Skywalker","height":"12"}}}`,
				expectedEstimatedCost: 22, // Query.hero (2) + Human.height (3) + Droid.name (17=max(7, 17))
			},
			computeCosts(),
		))

		t.Run("hero field has no weight (returns interface) and with concrete fragment", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero { name }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke Skywalker"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
							Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
								{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 7},
								{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 17},
							},
							Types: map[string]int{
								"Human": 13,
								"Droid": 11,
							},
						}},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
				expectedEstimatedCost: 30, // Query.Human (13) + Droid.name (17=max(7, 17))
			},
			computeCosts(),
		))

		t.Run("query hero without assumedSize on friends", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 1},
									{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 2},
									{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 2},
								},
								Types: map[string]int{
									"Human": 7,
									"Droid": 5,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
				expectedEstimatedCost: 127, // Query.hero(max(7,5))+10*(Human(max(7,5))+Human.name(2)+Human.height(1)+Droid.name(2))
			},
			computeCosts(),
		))

		t.Run("query hero with assumedSize on friends", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 1},
									{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 2},
									{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 5},
									{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 20},
								},
								Types: map[string]int{
									"Human": 7,
									"Droid": 5,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
				expectedEstimatedCost: 247, // Query.hero(max(7,5))+ 20 * (7+2+2+1)
				// We pick maximum on every path independently. This is to reveal the upper boundary.
				// Query.hero: picked maximum weight (Human=7) out of two types (Human, Droid)
				// Query.hero.friends: the max possible weight (7) is for implementing class Human
				// of the returned type of Character; the multiplier picked for the Droid since
				// it is the maximum possible value - we considered the enclosing type that contains it.
			},
			computeCosts(),
		))

		t.Run("query hero with assumedSize on friends and weight defined", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
							hero {
								friends {
									...on Droid { name primaryFunction }
									...on Human { name height }
								}
							}
						}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "friends"}: {HasWeight: true, Weight: 3},
									{TypeName: "Droid", FieldName: "friends"}: {HasWeight: true, Weight: 4},
									{TypeName: "Human", FieldName: "height"}:  {HasWeight: true, Weight: 1},
									{TypeName: "Human", FieldName: "name"}:    {HasWeight: true, Weight: 2},
									{TypeName: "Droid", FieldName: "name"}:    {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 5},
									{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 20},
								},
								Types: map[string]int{
									"Human": 7,
									"Droid": 5,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
				expectedEstimatedCost: 187, // Query.hero(max(7,5))+ 20 * (4+2+2+1)
			},
			computeCosts(),
		))

		t.Run("query hero with empty cost structures", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
									{"__typename":"Human","name":"Luke Skywalker","height":"12"},
									{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
								]}}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{},
						},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
				expectedEstimatedCost: 11, // Query.hero(max(1,1))+ 10 * 1
			},
			computeCosts(),
		))

		// Actual cost tests - verifies that actual cost uses real list sizes from response
		// rather than estimated/assumed sizes

		t.Run("actual cost with list field - 2 items instead of default 10", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero {
									friends {
										...on Droid { name primaryFunction }
										...on Human { name height }
									}
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// Response has 2 friends (not 10 as estimated)
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[
										{"__typename":"Human","name":"Luke Skywalker","height":"12"},
										{"__typename":"Droid","name":"R2DO","primaryFunction":"joke"}
									]}}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 1},
									{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 2},
									{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 2},
								},
								Types: map[string]int{
									"Human": 7,
									"Droid": 5,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"hero":{"friends":[{"name":"Luke Skywalker","height":"12"},{"name":"R2DO","primaryFunction":"joke"}]}}}`,
				// Estimated with default list size 10: hero(7) + 10 * (7 + 2 + 2 + 1) = 127
				expectedEstimatedCost: 127,
				// Actual uses real list size 2: hero(7) + 2 * (7 + 2 + 2 + 1) = 31
				expectedActualCost: 31,
			},
			computeCosts(),
		))

		t.Run("actual cost with empty list", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero {
									friends {
										...on Droid { name }
										...on Human { name }
									}
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// Response has empty friends array
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","friends":[]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 2},
									{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								Types: map[string]int{
									"Human": 7,
									"Droid": 5,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"hero":{"friends":[]}}}`,
				// Estimated with default list size 10: hero(7) + 10 * (7 + 2 + 2) = 117
				expectedEstimatedCost: 117,
				// Actual with empty list: hero(7) + 1 * (7 + 2 + 2) = 18
				// We consider empty lists as lists containing one item to account for the
				// resolver work.
				expectedActualCost: 18,
			},
			computeCosts(),
		))

		t.Run("named fragment on interface", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `
								fragment CharacterFields on Character {
									name
									friends { name }
								}
								{ hero { ...CharacterFields } }
								`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","friends":[{"name":"Leia"}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Query", FieldName: "hero"}: {HasWeight: true, Weight: 2},
									{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 3},
									{TypeName: "Droid", FieldName: "name"}: {HasWeight: true, Weight: 5},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Human", FieldName: "friends"}: {AssumedSize: 4},
									{TypeName: "Droid", FieldName: "friends"}: {AssumedSize: 6},
								},
								Types: map[string]int{
									"Human": 2,
									"Droid": 3,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"hero":{"name":"Luke","friends":[{"name":"Leia"}]}}}`,
				// Cost calculation:
				// Query.hero: 2
				// Character.name: max(Human.name=3, Droid.name=5) = 5
				//   friends listSize: max(4, 6) = 6
				//   Character type: max(Human=2, Droid=3) = 3
				//   name: max(Human.name=3, Droid.name=5) = 5
				// Total: 2 + 5 + 6 * (3 + 5)
				expectedEstimatedCost: 55,
			},
			computeCosts(),
		))

		t.Run("named fragment with concrete type", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `
								fragment HumanFields on Human {
									name
									height
								}
								{ hero { ...HumanFields } }
								`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","height":"1.72"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Query", FieldName: "hero"}:   {HasWeight: true, Weight: 2},
									{TypeName: "Human", FieldName: "name"}:   {HasWeight: true, Weight: 3},
									{TypeName: "Human", FieldName: "height"}: {HasWeight: true, Weight: 7},
									{TypeName: "Droid", FieldName: "name"}:   {HasWeight: true, Weight: 5},
								},
								Types: map[string]int{
									"Human": 1,
									"Droid": 1,
								},
							},
						},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"hero":{"name":"Luke","height":"1.72"}}}`,
				// Total: 2 + 3 + 7
				expectedEstimatedCost: 12,
			},
			computeCosts(),
		))

	})

	t.Run("union types", func(t *testing.T) {
		unionSchema := `
			type Query {
			   search(term: String!): [SearchResult!]
			}
			union SearchResult = User | Post | Comment
			type User @key(fields: "id") {
			  id: ID!
			  name: String!
			  email: String!
			}
			type Post @key(fields: "id") {
			  id: ID!
			  title: String!
			  body: String!
			}
			type Comment @key(fields: "id") {
			  id: ID!
			  text: String!
			}
			`
		schema, err := graphql.NewSchemaFromString(unionSchema)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"search"}},
			{TypeName: "User", FieldNames: []string{"id", "name", "email"}},
			{TypeName: "Post", FieldNames: []string{"id", "title", "body"}},
			{TypeName: "Comment", FieldNames: []string{"id", "text"}},
		}
		childNodes := []plan.TypeField{}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, unionSchema),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "term", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		t.Run("union with all member types", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  search(term: "test") {
							    ... on User { name email }
							    ... on Post { title body }
							    ... on Comment { text }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"search":[{"__typename":"User","name":"John","email":"john@test.com"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}:    {HasWeight: true, Weight: 2},
									{TypeName: "User", FieldName: "email"}:   {HasWeight: true, Weight: 3},
									{TypeName: "Post", FieldName: "title"}:   {HasWeight: true, Weight: 4},
									{TypeName: "Post", FieldName: "body"}:    {HasWeight: true, Weight: 5},
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {AssumedSize: 5},
								},
								Types: map[string]int{
									"User":    2,
									"Post":    3,
									"Comment": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"search":[{"name":"John","email":"john@test.com"}]}}`,
				// search listSize: 10
				// For each SearchResult, use max across all union members:
				//   Type weight: max(User=2, Post=3, Comment=1) = 3
				//   Fields: all fields from all fragments are counted
				//     (2 + 3) + (4 + 5) + (1) = 15
				// TODO: this is not correct, we should pick a maximum sum among types implementing union.
				//  9 should be used instead of 15
				// Total: 5 * (3 + 15)
				expectedEstimatedCost: 90,
			},
			computeCosts(),
		))

		t.Run("union with weighted search field", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  search(term: "test") {
							    ... on User { name }
							    ... on Post { title }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"search":[{"__typename":"User","name":"John"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}:  {HasWeight: true, Weight: 2},
									{TypeName: "Post", FieldName: "title"}: {HasWeight: true, Weight: 5},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {AssumedSize: 3},
								},
								Types: map[string]int{
									"User": 6,
									"Post": 10,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"search":[{"name":"John"}]}}`,
				// Query.search: max(User=10, Post=6)
				// search listSize: 3
				// Union members:
				//   All fields from all fragments: User.name(2) + Post.title(5)
				// Total: 3 * (10+2+5)
				// TODO: we might correct this by counting only members of one implementing types
				//  of a union when fragments are used.
				expectedEstimatedCost: 51,
			},
			computeCosts(),
		))
	})

	t.Run("listSize", func(t *testing.T) {
		listSchema := `
			type Query {
			   items(first: Int, last: Int): [Item!] 
			}
			type Item @key(fields: "id") {
			  id: ID
			} 
			`
		schemaSlicing, err := graphql.NewSchemaFromString(listSchema)
		require.NoError(t, err)
		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"items"}},
			{TypeName: "Item", FieldNames: []string{"id"}},
		}
		childNodes := []plan.TypeField{}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, listSchema),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "items",
				Path:      []string{"items"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "first",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
					{
						Name:         "last",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
				},
			},
		}
		t.Run("multiple slicing arguments as literals", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query MultipleSlicingArguments {
							  items(first: 5, last: 12) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "items"}: {
										AssumedSize:      8,
										SlicingArguments: []string{"first", "last"},
									},
								},
								Types: map[string]int{
									"Item": 3,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: 48, // slicingArgument(12) * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))
		t.Run("slicing argument as a variable", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query SlicingWithVariable($limit: Int!) {
							  items(first: $limit) { id }
							}`,
						Variables: []byte(`{"limit": 25}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "items"}: {
										AssumedSize:      8,
										SlicingArguments: []string{"first", "last"},
									},
								},
								Types: map[string]int{
									"Item": 3,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: 100, // slicingArgument($limit=25) * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))
		t.Run("slicing argument not provided falls back to assumedSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NoSlicingArg {
							  items { id }
							}`,
						// No slicing arguments provided - should fall back to assumedSize
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[{"id":"1"},{"id":"2"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "items"}: {
										AssumedSize:      15,
										SlicingArguments: []string{"first", "last"},
									},
								},
								Types: map[string]int{
									"Item": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"1"},{"id":"2"}]}}`,
				expectedEstimatedCost: 45, // Total: 15 * (2 + 1)
			},
			computeCosts(),
		))
		t.Run("zero slicing argument falls back to assumedSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query ZeroSlicing {
							  items(first: 0) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "items"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first", "last"},
									},
								},
								Types: map[string]int{
									"Item": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[]}}`,
				expectedEstimatedCost: 60, // 20 * (2 + 1)
			},
			computeCosts(),
		))
		t.Run("negative slicing argument falls back to assumedSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NegativeSlicing {
							  items(first: -5) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "items"}: {
										AssumedSize:      25,
										SlicingArguments: []string{"first", "last"},
									},
								},
								Types: map[string]int{
									"Item": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[]}}`,
				expectedEstimatedCost: 75, //  25 * (2 + 1)
			},
			computeCosts(),
		))

	})

	t.Run("nested lists with compounding multipliers", func(t *testing.T) {
		nestedSchema := `
			type Query {
			   users(first: Int): [User!]
			}
			type User @key(fields: "id") {
			  id: ID!
			  posts(first: Int): [Post!]
			}
			type Post @key(fields: "id") {
			  id: ID!
			  comments(first: Int): [Comment!]
			}
			type Comment @key(fields: "id") {
			  id: ID!
			  text: String!
			}
			`
		schemaNested, err := graphql.NewSchemaFromString(nestedSchema)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"users"}},
			{TypeName: "User", FieldNames: []string{"id", "posts"}},
			{TypeName: "Post", FieldNames: []string{"id", "comments"}},
			{TypeName: "Comment", FieldNames: []string{"id", "text"}},
		}
		childNodes := []plan.TypeField{}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, nestedSchema),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName: "Query", FieldName: "users", Path: []string{"users"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "User", FieldName: "posts", Path: []string{"posts"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Post", FieldName: "comments", Path: []string{"comments"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		t.Run("nested lists with slicing arguments", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 3) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      50,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
				// Cost calculation:
				// users(first:10): multiplier 10
				//   User type weight: 4
				//   posts(first:5): multiplier 5
				//     Post type weight: 3
				//     comments(first:3): multiplier 3
				//       Comment type weight: 2
				//       text weight: 1
				// Total: 10 * (4 + 5 * (3 + 3 * (2 + 1)))
				expectedEstimatedCost: 640,
			},
			computeCosts(),
		))

		t.Run("nested lists fallback to assumedSize when slicing arg not provided", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 2) {
							    posts {
							      comments(first: 4) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":[{"posts":[{"comments":[{"text":"hi"}]}]}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize: 50, // no slicing arg, should use this
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"hi"}]}]}]}}`,
				// Cost calculation:
				// users(first:2): multiplier 2
				//   User type weight: 4
				//   posts (no arg): assumedSize 50
				//     Post type weight: 3
				//     comments(first:4): multiplier 4
				//       Comment type weight: 2
				//       text weight: 1
				// Total: 2 * (4 + 50 * (3 + 4 * (2 + 1)))
				expectedEstimatedCost: 1508,
			},
			computeCosts(),
		))

		t.Run("actual cost for nested lists - 1 item at each level", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 3) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								// Response has 1 user with 1 post with 1 comment
								sendResponseBody: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      50,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"hello"}]}]}]}}`,
				// Estimated cost with slicing arguments (10, 5, 3):
				// Total: 10 * (4 + 5 * (3 + 3 * (2 + 1))) = 640
				expectedEstimatedCost: 640,
				// Actual cost with 1 item at each level:
				// Total: 1 * (4 + 1 * (3 + 1 * (2 + 1))) = 10
				expectedActualCost: 10,
			},
			computeCosts(),
		))

		t.Run("actual cost for nested lists - varying sizes", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 3) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								// Response has 2 users, each with 2 posts, each with 3 comments
								sendResponseBody: `{"data":{"users":[
										{"posts":[
											{"comments":[{"text":"a"},{"text":"b"},{"text":"c"}]},
											{"comments":[{"text":"d"},{"text":"e"},{"text":"f"}]}]},
										{"posts":[
											{"comments":[{"text":"g"},{"text":"h"},{"text":"i"}]},
											{"comments":[{"text":"j"},{"text":"k"},{"text":"l"}]}]}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      50,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"users":[{"posts":[{"comments":[{"text":"a"},{"text":"b"},{"text":"c"}]},{"comments":[{"text":"d"},{"text":"e"},{"text":"f"}]}]},{"posts":[{"comments":[{"text":"g"},{"text":"h"},{"text":"i"}]},{"comments":[{"text":"j"},{"text":"k"},{"text":"l"}]}]}]}}`,
				expectedEstimatedCost: 640,
				// Actual cost: 2 * (4 + 2 * (3 + 3 * (2 + 1))) = 56
				expectedActualCost: 56,
			},
			computeCosts(),
		))

		t.Run("actual cost for nested lists - uneven sizes", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 2) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								// Response has 2 users, with 1.5 posts each, each with 3 comments
								sendResponseBody: `{"data":{"users":[
										{"posts":[
											{"comments":[{"text":"d"},{"text":"e"},{"text":"f"}]}]},
										{"posts":[
											{"comments":[{"text":"g"},{"text":"h"},{"text":"i"}]},
											{"comments":[{"text":"j"},{"text":"k"},{"text":"l"}]}]}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      50,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":[{"posts":[{"comments":[{"text":"d"},{"text":"e"},{"text":"f"}]}]},{"posts":[{"comments":[{"text":"g"},{"text":"h"},{"text":"i"}]},{"comments":[{"text":"j"},{"text":"k"},{"text":"l"}]}]}]}}`,
				// Estimated : 10 * (4 + 5 * (3 + 2 * (2 + 1))) = 490
				expectedEstimatedCost: 490,
				// Actual cost: 2 * (4 + 1.5 * (3 + 3 * (2 + 1))) = 44
				expectedActualCost: 44,
			},
			computeCosts(),
		))

		t.Run("actual cost for root-level list - no parent", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 10) { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								// Response has 3 users at the root level
								sendResponseBody: `{"data":{"users":[
										{"id":"1"},
										{"id":"2"},
										{"id":"3"}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User": 4,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":[{"id":"1"},{"id":"2"},{"id":"3"}]}}`,
				// Estimated: 10 * (4 + 1) = 50
				expectedEstimatedCost: 50,
				// Actual cost: 3 users at root
				// 3 * (4 + 1) = 15
				expectedActualCost: 15,
			},
			computeCosts(),
		))

		t.Run("mixed empty and non-empty lists - averaging behavior", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaNested,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  users(first: 10) {
							    posts(first: 5) {
							      comments(first: 3) { text }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								sendResponseBody: `{"data":{"users":[
										{"posts":[
											{"comments":[{"text":"a"},{"text":"b"}]},
											{"comments":[{"text":"c"},{"text":"d"}]}
										]},
										{"posts":[]},
										{"posts":[
											{"comments":[]}
										]}
									]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Comment", FieldName: "text"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      50,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Post", FieldName: "comments"}: {
										AssumedSize:      20,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"User":    4,
									"Post":    3,
									"Comment": 2,
								},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"users":[{"posts":[{"comments":[{"text":"a"},{"text":"b"}]},{"comments":[{"text":"c"},{"text":"d"}]}]},{"posts":[]},{"posts":[{"comments":[]}]}]}}`,
				expectedEstimatedCost: 640, // 10 * (4 + 5 * (3 + 3 * (2 + 1)))
				// Actual cost with mixed empty/non-empty lists:
				// Users: 3 items, multiplier 3.0
				// Posts: 3 items, 3 parents => multiplier 1.0 (avg)
				// Comments: 4 items, 3 parents => multiplier 1.33 (avg)
				//
				// Calculation:
				// Comments: RoundToEven((2 + 1) * 1.33) ~= 4
				// Posts:    RoundToEven((3 + 4) * 1.00)  = 7
				// Users:    RoundToEven((4 + 7) * 3.00)  = 33
				//
				// Empty lists are included in the averaging:
				expectedActualCost: 33,
			},
			computeCosts(),
		))

		t.Run("deeply nested lists with fractional multipliers - compounding rounding", runWithoutError(
			ExecutionEngineTestCase{
				schema: func() *graphql.Schema {
					deepSchema := `
						type Query {
						   level1(first: Int): [Level1!]
						}
						type Level1 @key(fields: "id") {
						  id: ID!
						  level2(first: Int): [Level2!]
						}
						type Level2 @key(fields: "id") {
						  id: ID!
						  level3(first: Int): [Level3!]
						}
						type Level3 @key(fields: "id") {
						  id: ID!
						  level4(first: Int): [Level4!]
						}
						type Level4 @key(fields: "id") {
						  id: ID!
						  level5(first: Int): [Level5!]
						}
						type Level5 @key(fields: "id") {
						  id: ID!
						  value: String!
						}
						`
					s, err := graphql.NewSchemaFromString(deepSchema)
					require.NoError(t, err)
					return s
				}(),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{
							  level1(first: 10) {
							    level2(first: 10) {
							      level3(first: 10) {
							        level4(first: 10) {
							          level5(first: 10) {
							            value
							          }
							        }
							      }
							    }
							  }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								sendResponseBody: `{"data":{"level1":[
										{"level2":[
											{"level3":[
												{"level4":[
													{"level5":[{"value":"a"}]},
													{"level5":[{"value":"b"},{"value":"c"}]}
												]},
												{"level4":[
													{"level5":[{"value":"d"}]}
												]}
											]},
											{"level3":[
												{"level4":[
													{"level5":[{"value":"e"}]}
												]}
											]}
										]},
										{"level2":[
											{"level3":[
												{"level4":[
													{"level5":[{"value":"f"},{"value":"g"}]},
													{"level5":[{"value":"h"}]}
												]},
												{"level4":[
													{"level5":[{"value":"i"}]}
												]}
											]}
										]},
										{"level2":[
											{"level3":[
												{"level4":[
													{"level5":[{"value":"j"}]},
													{"level5":[{"value":"k"}]}
												]},
												{"level4":[
													{"level5":[{"value":"l"}]},
													{"level5":[{"value":"m"}]}
												]}
											]}
										]}
									]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes: []plan.TypeField{
								{TypeName: "Query", FieldNames: []string{"level1"}},
								{TypeName: "Level1", FieldNames: []string{"id", "level2"}},
								{TypeName: "Level2", FieldNames: []string{"id", "level3"}},
								{TypeName: "Level3", FieldNames: []string{"id", "level4"}},
								{TypeName: "Level4", FieldNames: []string{"id", "level5"}},
								{TypeName: "Level5", FieldNames: []string{"id", "value"}},
							},
							ChildNodes: []plan.TypeField{},
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "Level5", FieldName: "value"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "level1"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Level1", FieldName: "level2"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Level2", FieldName: "level3"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Level3", FieldName: "level4"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
									{TypeName: "Level4", FieldName: "level5"}: {
										AssumedSize:      100,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"Level1": 1,
									"Level2": 1,
									"Level3": 1,
									"Level4": 1,
									"Level5": 1,
								},
							},
						},
						mustConfiguration(t, graphql_datasource.ConfigurationInput{
							Fetch: &graphql_datasource.FetchConfiguration{
								URL:    "https://example.com/",
								Method: "GET",
							},
							SchemaConfiguration: mustSchemaConfig(t, nil, `
									type Query {
									   level1(first: Int): [Level1!]
									}
									type Level1 @key(fields: "id") {
									  id: ID!
									  level2(first: Int): [Level2!]
									}
									type Level2 @key(fields: "id") {
									  id: ID!
									  level3(first: Int): [Level3!]
									}
									type Level3 @key(fields: "id") {
									  id: ID!
									  level4(first: Int): [Level4!]
									}
									type Level4 @key(fields: "id") {
									  id: ID!
									  level5(first: Int): [Level5!]
									}
									type Level5 @key(fields: "id") {
									  id: ID!
									  value: String!
									}
								`),
						}),
					),
				},
				fields: []plan.FieldConfiguration{
					{
						TypeName: "Query", FieldName: "level1", Path: []string{"level1"},
						Arguments: []plan.ArgumentConfiguration{
							{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						},
					},
					{
						TypeName: "Level1", FieldName: "level2", Path: []string{"level2"},
						Arguments: []plan.ArgumentConfiguration{
							{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						},
					},
					{
						TypeName: "Level2", FieldName: "level3", Path: []string{"level3"},
						Arguments: []plan.ArgumentConfiguration{
							{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						},
					},
					{
						TypeName: "Level3", FieldName: "level4", Path: []string{"level4"},
						Arguments: []plan.ArgumentConfiguration{
							{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						},
					},
					{
						TypeName: "Level4", FieldName: "level5", Path: []string{"level5"},
						Arguments: []plan.ArgumentConfiguration{
							{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						},
					},
				},
				expectedResponse:      `{"data":{"level1":[{"level2":[{"level3":[{"level4":[{"level5":[{"value":"a"}]},{"level5":[{"value":"b"},{"value":"c"}]}]},{"level4":[{"level5":[{"value":"d"}]}]}]},{"level3":[{"level4":[{"level5":[{"value":"e"}]}]}]}]},{"level2":[{"level3":[{"level4":[{"level5":[{"value":"f"},{"value":"g"}]},{"level5":[{"value":"h"}]}]},{"level4":[{"level5":[{"value":"i"}]}]}]}]},{"level2":[{"level3":[{"level4":[{"level5":[{"value":"j"}]},{"level5":[{"value":"k"}]}]},{"level4":[{"level5":[{"value":"l"}]},{"level5":[{"value":"m"}]}]}]}]}]}}`,
				expectedEstimatedCost: 211110,
				// Actual cost with fractional multipliers:
				// Level5: 13 items, 11 parents => multiplier 1.18 (13/11 = 1.181818...)
				// Level4: 11 items,  7 parents => multiplier 1.57 (11/7 = 1.571428...)
				// Level3:  7 items,  4 parents => multiplier 1.75 (7/4 = 1.75)
				// Level2:  4 items,  3 parents => multiplier 1.33 (4/3 = 1.333...)
				// Level1:  3 items,  1 parent  => multiplier 3.0
				//
				// Ideal calculation without rounding:
				// cost = 3 * (1 + 1.33 * (1 + 1.75 * (1 + 1.57 * (1 + 1.18 * (1 + 1)))))
				//      = 50.806584 ~= 51
				//
				// Current implementation:
				// Level5: RoundToEven((1 +  1) * 1.18) = 2
				// Level4: RoundToEven((1 +  2) * 1.57) = 5
				// Level3: RoundToEven((1 +  5) * 1.75) = 10 (rounds to even)
				// Level2: RoundToEven((1 + 10) * 1.33) = 15
				// Level1: RoundToEven((1 + 15) * 3.00) = 48
				//
				// The compounding rounding error: 48 vs 51 (6% underestimate)
				expectedActualCost: 48,
			},
			computeCosts(),
		))
	})

	t.Run("sizedFields", func(t *testing.T) {
		connSchema := `
			type Query {
				users(first: Int, last: Int): UserConnection!
			}
			type UserConnection {
				edges: [UserEdge!]
				nodes: [User!]
				totalCount: Int!
			}
			type UserEdge {
				cursor: String!
				node: User!
			}
			type User @key(fields: "id") {
				id: ID!
				name: String!
				posts(first: Int): [Post!]
			}
			type Post @key(fields: "id") {
				id: ID!
				title: String!
			}
			`
		schemaConn, err := graphql.NewSchemaFromString(connSchema)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"users"}},
			{TypeName: "User", FieldNames: []string{"id", "name", "posts"}},
			{TypeName: "Post", FieldNames: []string{"id", "title"}},
			{TypeName: "UserConnection", FieldNames: []string{"edges", "nodes", "totalCount"}},
			{TypeName: "UserEdge", FieldNames: []string{"cursor", "node"}},
		}
		childNodes := []plan.TypeField{}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, connSchema),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName: "Query", FieldName: "users", Path: []string{"users"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "User", FieldName: "posts", Path: []string{"posts"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		t.Run("with cursor pattern - slicing argument", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 5, last: 8) { 
										edges { 
										  node { name } 
										} 
										nodes { name } 
										totalCount } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}],"nodes":[{"name":"Alice"}],"totalCount":1}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first", "last"},
										SizedFields:      []string{"edges", "nodes"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}],"nodes":[{"name":"Alice"}],"totalCount":1}}}`,
				// UserConnection(1) + Int(0) + 8*(UserEdge(1)+User(1)+User.name(2)) + 8*(User(1)+User.name(2))
				expectedEstimatedCost: 57,
				// UserConnection(1) + Int(0) + 1*(UserEdge(1)+User(1)+User.name(2)) + 1*(User(1)+User.name(2))
				expectedActualCost: 8,
			},
			computeCosts(),
		))

		t.Run("with assumedSize fallback", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users { edges { node { name } } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										AssumedSize:      3,
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
				// UserConnection(1) + 3*(UserEdge(1)+User(1)+User.name(2))
				expectedEstimatedCost: 13,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: 5,
			},
			computeCosts(),
		))

		t.Run("child with its own listSize is not overridden", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 5) { 
								edges { 
									node { 
										name 
										posts(first: 2) { title } 
									} 
								} } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice","posts":[{"title":"Hello"}]}}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}:  {HasWeight: true, Weight: 2},
									{TypeName: "Post", FieldName: "title"}: {HasWeight: true, Weight: 3},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
									{TypeName: "User", FieldName: "posts"}: {
										AssumedSize:      10,
										SlicingArguments: []string{"first"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
									"Post":     1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice","posts":[{"title":"Hello"}]}}]}}}`,
				// UserConnection(1) + 5*(UserEdge(1)+User(1)+User.name(2)+2*(Post(1)+Post.title(3)))
				expectedEstimatedCost: 61,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2)+1*(Post(1)+Post.title(3)))
				expectedActualCost: 9,
			},
			computeCosts(),
		))

		t.Run("direct child with its own listSize is not overridden", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 5) { 
								edges { 
									node { 
										name 
									} 
								} } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
									{TypeName: "UserConnection", FieldName: "edges"}: {
										AssumedSize: 10,
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
				// UserConnection(1) + 10*(UserEdge(1)+User(1)+User.name(2)))
				expectedEstimatedCost: 41,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: 5,
			},
			computeCosts(),
		))

		t.Run("sizedFields with no matching child queried", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 5) { totalCount } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"totalCount":42}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges", "nodes"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"totalCount":42}}}`,
				// UserConnection(1) + Int(0) = 1
				expectedEstimatedCost: 1,
				expectedActualCost:    1,
			},
			computeCosts(),
		))

		t.Run("sizedFields with variable slicing argument", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query:     `query($n: Int) { users(first: $n) { edges { node { name } } } }`,
						Variables: []byte(`{"n": 7}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
				// UserConnection(1) + 7*(UserEdge(1)+User(1)+User.name(2))
				expectedEstimatedCost: 29,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: 5,
			},
			computeCosts(),
		))

		t.Run("sizedFields fallback to defaultListSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users { edges { node { name } } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
				// No slicing arg provided, no AssumedSize -> falls back to defaultListSize(10)
				// UserConnection(1) + 10*(UserEdge(1)+User(1)+User.name(2))
				expectedEstimatedCost: 41,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: 5,
			},
			computeCosts(),
		))

		t.Run("mixed sizedFields and non-sizedFields list children", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaConn,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ users(first: 5) { edges { node { name } } nodes { name } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost:     "example.com",
								expectedPath:     "/",
								expectedBody:     "",
								sendResponseBody: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}],"nodes":[{"name":"Alice"}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
									{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "users"}: {
										SlicingArguments: []string{"first"},
										SizedFields:      []string{"edges"},
									},
								},
								Types: map[string]int{
									"UserEdge": 1,
								},
							},
						},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}],"nodes":[{"name":"Alice"}]}}}`,
				// edges is a sizedField -> multiplier from parent slicing arg = 5
				// nodes is NOT a sizedField -> falls back to defaultListSize(10)
				// UserConnection(1) + 5*(UserEdge(1)+User(1)+User.name(2)) + 10*(User(1)+User.name(2))
				expectedEstimatedCost: 51,
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2)) + 1*(User(1)+User.name(2))
				expectedActualCost: 8,
			},
			computeCosts(),
		))
	})

	t.Run("sizedFields on abstract types", func(t *testing.T) {
		t.Run("parent returns interface, child via inline fragment", func(t *testing.T) {
			s2Schema := `
					interface Connection {
						edges: [Edge]
					}
					type Edge {
						cursor: String
					}
					type UserConnection implements Connection {
						edges: [UserEdge]
					}
					type UserEdge {
						cursor: String
						node: User
					}
					type User @key(fields: "id") {
						id: ID!
						name: String!
					}
					type Query {
						users(first: Int): Connection
					}
					`
			schema, err := graphql.NewSchemaFromString(s2Schema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"users"}},
				{TypeName: "User", FieldNames: []string{"id", "name"}},
				{TypeName: "UserConnection", FieldNames: []string{"edges"}},
				{TypeName: "UserEdge", FieldNames: []string{"cursor", "node"}},
				{TypeName: "Edge", FieldNames: []string{"cursor"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Connection", FieldNames: []string{"edges"}},
			}
			customCfg := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, s2Schema),
			})
			fields := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "users", Path: []string{"users"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("edges via inline fragment", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ users(first: 3) { ... on UserConnection { edges { node { name } } } } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"users":{"__typename":"UserConnection","edges":[{"node":{"name":"Alice"}}]}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
										{TypeName: "User", FieldName: "name"}: {HasWeight: true, Weight: 2},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "users"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"edges"},
										},
									},
									Types: map[string]int{
										"UserEdge": 3,
									},
								},
							},
							customCfg,
						),
					},
					fields:           fields,
					expectedResponse: `{"data":{"users":{"edges":[{"node":{"name":"Alice"}}]}}}`,
					// max(Connection,UserConnection)(1) + 3*(UserEdge(3)+User(1)+User.name(2))
					expectedEstimatedCost: 19,
					expectedActualCost:    7,
				},
				computeCosts(),
			))
		})

		t.Run("parent returns interface, child accessed directly", func(t *testing.T) {
			s3Schema := `
					interface Connection {
						edges: [Edge]
					}
					type Edge {
						cursor: String
					}
					type Query {
						users(first: Int): Connection
					}
					`
			schema, err := graphql.NewSchemaFromString(s3Schema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"users"}},
				{TypeName: "Edge", FieldNames: []string{"cursor"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Connection", FieldNames: []string{"edges"}},
			}
			customCfg := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, s3Schema),
			})
			fields := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "users", Path: []string{"users"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("edges accessed directly", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ users(first: 4) { edges { cursor } } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"users":{"edges":[{"cursor":"abc"}]}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "users"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"edges"},
										},
									},
									Types: map[string]int{
										"Edge": 3,
									},
								},
							},
							customCfg,
						),
					},
					fields:           fields,
					expectedResponse: `{"data":{"users":{"edges":[{"cursor":"abc"}]}}}`,
					// Connection(1) + 4*(Edge(3)+String(0))
					expectedEstimatedCost: 13,
					expectedActualCost:    4,
				},
				computeCosts(),
			))
		})

		t.Run("sizedFields on interface field", func(t *testing.T) {
			s4Schema := `
					interface Paginated {
						items(first: Int): ItemConnection
					}
					type UserPaginated implements Paginated {
						items(first: Int): ItemConnection
					}
					type ItemConnection {
						edges: [ItemEdge]
					}
					type ItemEdge {
						cursor: String
						node: Item
					}
					type Item @key(fields: "id") {
						id: ID!
						name: String!
					}
					type Query {
						search: Paginated
					}
					`
			schema, err := graphql.NewSchemaFromString(s4Schema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
				{TypeName: "UserPaginated", FieldNames: []string{"items"}},
				{TypeName: "Item", FieldNames: []string{"id", "name"}},
				{TypeName: "ItemConnection", FieldNames: []string{"edges"}},
				{TypeName: "ItemEdge", FieldNames: []string{"cursor", "node"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Paginated", FieldNames: []string{"items"}},
			}
			customCfg := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, s4Schema),
			})
			fields := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "search", Path: []string{"search"},
				},
				{
					TypeName: "Paginated", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "UserPaginated", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("on interface field", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ search { items(first: 5) { edges { node { name } } } } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"search":{"__typename":"UserPaginated","items":{"edges":[{"node":{"name":"Alice"}}]}}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									// @listSize on the INTERFACE field Paginated.items
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Paginated", FieldName: "items"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"edges"},
										},
									},
									Types: map[string]int{
										"ItemEdge": 2,
									},
								},
							},
							customCfg,
						),
					},
					fields:           fields,
					expectedResponse: `{"data":{"search":{"items":{"edges":[{"node":{"name":"Alice"}}]}}}}`,
					// Paginated(max(1,1)) + ItemConnection(1) + 5*(ItemEdge(2)+Item(1)+Item.name(0))
					expectedEstimatedCost: 17,
					expectedActualCost:    5,
				},
				computeCosts(),
			))
		})

		t.Run("sizedFields only on concrete types, accessed through interface", func(t *testing.T) {
			s5Schema := `
					interface Paginated {
						items(first: Int): ItemConnection
					}
					type UserPaginated implements Paginated {
						items(first: Int): ItemConnection
					}
					type PostPaginated implements Paginated {
						items(first: Int): ItemConnection
					}
					type ItemConnection {
						edges: [ItemEdge]
					}
					type ItemEdge {
						cursor: String
						node: Item
					}
					type Item @key(fields: "id") {
						id: ID!
						name: String!
					}
					type Query {
						search: Paginated
					}
					`
			schema, err := graphql.NewSchemaFromString(s5Schema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"search"}},
				{TypeName: "UserPaginated", FieldNames: []string{"items"}},
				{TypeName: "PostPaginated", FieldNames: []string{"items"}},
				{TypeName: "Item", FieldNames: []string{"id", "name"}},
				{TypeName: "ItemConnection", FieldNames: []string{"edges"}},
				{TypeName: "ItemEdge", FieldNames: []string{"cursor", "node"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Paginated", FieldNames: []string{"items"}},
			}
			customCfg := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, s5Schema),
			})
			fields := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "search", Path: []string{"search"},
				},
				{
					TypeName: "Paginated", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "UserPaginated", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "PostPaginated", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("sizedFields on concrete", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ search { items(first: 5) { edges { node { name } } } } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"search":{"__typename":"UserPaginated","items":{"edges":[{"node":{"name":"Alice"}}]}}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									// @listSize on CONCRETE types only, NOT on Paginated.items
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "UserPaginated", FieldName: "items"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"edges"},
										},
										{TypeName: "PostPaginated", FieldName: "items"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"edges"},
										},
									},
									Types: map[string]int{
										"ItemEdge": 3,
									},
								},
							},
							customCfg,
						),
					},
					fields:           fields,
					expectedResponse: `{"data":{"search":{"items":{"edges":[{"node":{"name":"Alice"}}]}}}}`,
					// Estimated cost should be 22 = Paginated + ItemConnection + 5*(ItemEdge+Item),
					// Parent fieldCoord == {Paginated, items},
					// ListSizes only has {UserPaginated, items} and {PostPaginated, items}.
					// If not considering implementations, multiplier for edges falls back to
					// defaultListSize(10): 1 + 1 + 10*(3+1) = 42.
					expectedEstimatedCost: 22,
					expectedActualCost:    6,
				},
				computeCosts(),
			))
		})

		t.Run("sizedField returns list of abstract type", func(t *testing.T) {
			s7Schema := `
					interface Publishable {
						id: ID!
					}
					type Post implements Publishable {
						id: ID!
						title: String!
					}
					type FeedConnection {
						items: [Publishable]
						count: Int
					}
					type Query {
						feed(first: Int): FeedConnection
					}
					`
			schema, err := graphql.NewSchemaFromString(s7Schema)
			require.NoError(t, err)

			rootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"feed"}},
				{TypeName: "Post", FieldNames: []string{"id", "title"}},
				{TypeName: "FeedConnection", FieldNames: []string{"items", "count"}},
			}
			childNodes := []plan.TypeField{
				{TypeName: "Publishable", FieldNames: []string{"id"}},
			}
			customCfg := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, s7Schema),
			})
			fields := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "feed", Path: []string{"feed"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}

			t.Run("items returns list of Publishable interface", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ feed(first: 3) { items { id } count } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"feed":{"items":[{"id":"1"},{"id":"2"}],"count":2}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "feed"}: {
											SlicingArguments: []string{"first"},
											SizedFields:      []string{"items"},
										},
									},
									Types: map[string]int{
										"Post": 3,
									},
								},
							},
							customCfg,
						),
					},
					fields:           fields,
					expectedResponse: `{"data":{"feed":{"items":[{"id":"1"},{"id":"2"}],"count":2}}}`,
					// FeedConnection(1) + Int(0) + 3*(max(Post(3))+ID(0))
					expectedEstimatedCost: 10,
					expectedActualCost:    7,
				},
				computeCosts(),
			))
		})

	})

	t.Run("validate requireOneSlicingArgument on concrete types", func(t *testing.T) {
		listSchema := `
			type Query {
			   items(first: Int, last: Int): [Item!] # @listSize(assumedSize: 10, SlicingArguments: ["first", "last"], RequireOneSlicingArgument: true/false)
			   itemsNoSlicing: [Item!]  # @listSize(assumedSize: 5, RequireOneSlicingArgument: true)
			}
			type Item @key(fields: "id") {
			  id: ID
			}
			`
		schema, err := graphql.NewSchemaFromString(listSchema)
		require.NoError(t, err)
		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"items", "itemsNoSlicing"}},
			{TypeName: "Item", FieldNames: []string{"id"}},
		}
		childNodes := []plan.TypeField{}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, listSchema),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "items",
				Path:      []string{"items"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "first",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
					{
						Name:         "last",
						SourceType:   plan.FieldArgumentSource,
						RenderConfig: plan.RenderArgumentAsGraphQLValue,
					},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "itemsNoSlicing",
				Path:      []string{"itemsNoSlicing"},
			},
		}

		costConfigWithRequireOne := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
				{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "items"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: true,
				},
				{TypeName: "Query", FieldName: "itemsNoSlicing"}: {
					AssumedSize:               5,
					RequireOneSlicingArgument: true,
				},
			},
			Types: map[string]int{
				"Item": 2,
			},
		}

		costConfigWithRequireOneDisabled := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldWeight{
				{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "items"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: false,
				},
			},
			Types: map[string]int{
				"Item": 2,
			},
		}

		t.Run("no slicingArguments defined - requireOneSlicingArgument ignored", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ itemsNoSlicing { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"itemsNoSlicing":[{"id":"1"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOne,
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"itemsNoSlicing":[{"id":"1"}]}}`,
				expectedEstimatedCost: 15, // assumedSize(5) * (Item(2) + Item.id(1))
			},
			computeCosts(),
		))

		t.Run("exactly one slicing argument provided - valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ items(first: 4) { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[{"id":"1"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOne,
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"1"}]}}`,
				expectedEstimatedCost: 12, // 4 * (Item(2) + Item.id(1))
			},
			computeCosts(),
		))

		t.Run("no slicing argument provided - error", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ items { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOne,
						},
						customConfig,
					),
				},
				fields: fieldConfig,
			},
			"external: field 'Query.items' requires exactly one slicing argument, but none was provided, locations: [], path: [items]",
			computeCosts(),
		))

		t.Run("multiple slicing arguments provided - error", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ items(first: 5, last: 3) { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOne,
						},
						customConfig,
					),
				},
				fields: fieldConfig,
			},
			"external: field 'Query.items' requires exactly one slicing argument, but 2 were provided, locations: [], path: [items]",
			computeCosts(),
		))

		t.Run("no slicing argument but requireOneSlicingArgument disabled - valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ items { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[{"id":"1"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOneDisabled,
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"1"}]}}`,
				expectedEstimatedCost: 30, // assumedSize(10) * (Item(2) + Item.id(1))
			},
			computeCosts(),
		))

		t.Run("multiple slicing arguments but requireOneSlicingArgument disabled - valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ items(first: 5, last: 3) { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[{"id":"1"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOneDisabled,
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"1"}]}}`,
				expectedEstimatedCost: 15, // max(5,3)=5 * (Item(2) + Item.id(1))
			},
			computeCosts(),
		))

		t.Run("slicing argument provided as variable - valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query:     `query ($n: Int!) { items(first: $n) { id } }`,
						Variables: []byte(`{"n": 7}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"items":[{"id":"1"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: costConfigWithRequireOne,
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"items":[{"id":"1"}]}}`,
				expectedEstimatedCost: 21, // 7 * (Item(2) + Item.id(1))
			},
			computeCosts(),
		))

		t.Run("multiple fields violating - collects all errors", func(t *testing.T) {
			multiSchema := `
				type Query {
					items(first: Int, last: Int): [Item!] # @listSize(assumedSize: 10, slicingArguments: ["first", "last"], requireOneSlicingArgument: true)
					other(first: Int, last: Int): [Item!] # @listSize(assumedSize: 10, slicingArguments: ["first", "last"], requireOneSlicingArgument: true)
				}
				type Item @key(fields: "id") @cost(weight: 2) {
					id: ID
				}
			`
			multiSchemaObj, err := graphql.NewSchemaFromString(multiSchema)
			require.NoError(t, err)
			multiRootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"items", "other"}},
				{TypeName: "Item", FieldNames: []string{"id"}},
			}
			multiCustomConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, multiSchema),
			})
			multiFieldConfig := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "items", Path: []string{"items"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "Query", FieldName: "other", Path: []string{"other"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}
			multiCostConfig := &plan.DataSourceCostConfig{
				ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
					{TypeName: "Query", FieldName: "items"}: {
						AssumedSize: 10, SlicingArguments: []string{"first", "last"}, RequireOneSlicingArgument: true,
					},
					{TypeName: "Query", FieldName: "other"}: {
						AssumedSize: 10, SlicingArguments: []string{"first", "last"}, RequireOneSlicingArgument: true,
					},
				},
				Types: map[string]int{"Item": 2},
			}

			t.Run("both fields missing slicing argument", runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: multiSchemaObj,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{ items { id } other { id } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"items":[],"other":[]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  multiRootNodes,
								CostConfig: multiCostConfig,
							},
							multiCustomConfig,
						),
					},
					fields: multiFieldConfig,
				},
				"external: field 'Query.items' requires exactly one slicing argument, but none was provided, locations: [], path: [items]\n"+
					"external: field 'Query.other' requires exactly one slicing argument, but none was provided, locations: [], path: [other]",
				computeCosts(),
			))
		})
	})
	t.Run("validate requireOneSlicingArgument on abstract types", func(t *testing.T) {
		// Abstract type tests: @listSize with requireOneSlicingArgument on concrete types,
		// accessed through an interface field.
		abstractSchema := `
			interface Paginated {
				items(first: Int, last: Int): [Item!]
			}
			type UserPaginated implements Paginated {
				items(first: Int, last: Int): [Item!] # @listSize(assumedSize: 10, SlicingArguments: ["first", "last"], RequireOneSlicingArgument: true)
			}
			type PostPaginated implements Paginated {
				items(first: Int, last: Int): [Item!] # @listSize(assumedSize: 10, SlicingArguments: ["first", "last"], RequireOneSlicingArgument: false)
			}
			type Item @key(fields: "id") {
				id: ID!
			}
			type Query {
				search: Paginated
			}
			`
		abstractSchemaObj, err := graphql.NewSchemaFromString(abstractSchema)
		require.NoError(t, err)
		abstractRootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"search"}},
			{TypeName: "UserPaginated", FieldNames: []string{"items"}},
			{TypeName: "PostPaginated", FieldNames: []string{"items"}},
			{TypeName: "Item", FieldNames: []string{"id"}},
		}
		abstractChildNodes := []plan.TypeField{
			{TypeName: "Paginated", FieldNames: []string{"items"}},
		}
		abstractCustomConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, abstractSchema),
		})
		abstractFieldConfig := []plan.FieldConfiguration{
			{
				TypeName: "Query", FieldName: "search", Path: []string{"search"},
			},
			{
				TypeName: "Paginated", FieldName: "items", Path: []string{"items"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "UserPaginated", FieldName: "items", Path: []string{"items"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "PostPaginated", FieldName: "items", Path: []string{"items"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}
		abstractCostConfig := &plan.DataSourceCostConfig{
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "UserPaginated", FieldName: "items"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: false,
				},
				{TypeName: "PostPaginated", FieldName: "items"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: true,
				},
			},
			Types: map[string]int{
				"Item": 2,
			},
		}

		t.Run("abstract type - exactly one slicing argument - valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: abstractSchemaObj,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ search { items(first: 5) { id } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":{"__typename":"UserPaginated","items":[{"id":"1"}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  abstractRootNodes,
							ChildNodes: abstractChildNodes,
							CostConfig: abstractCostConfig,
						},
						abstractCustomConfig,
					),
				},
				fields:                abstractFieldConfig,
				expectedResponse:      `{"data":{"search":{"items":[{"id":"1"}]}}}`,
				expectedEstimatedCost: 11, // Paginated(1) + 5 * (Item(2) + Item.id(0))
			},
			computeCosts(),
		))

		t.Run("abstract type - no slicing argument - error", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: abstractSchemaObj,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ search { items { id } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":{"__typename":"UserPaginated","items":[]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  abstractRootNodes,
							ChildNodes: abstractChildNodes,
							CostConfig: abstractCostConfig,
						},
						abstractCustomConfig,
					),
				},
				fields: abstractFieldConfig,
			},
			"external: field 'Paginated.items' requires exactly one slicing argument, but none was provided, locations: [], path: [search,items]",
			computeCosts(),
		))

		t.Run("abstract type - multiple slicing arguments - error", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: abstractSchemaObj,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ search { items(first: 5, last: 3) { id } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":{"__typename":"UserPaginated","items":[]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  abstractRootNodes,
							ChildNodes: abstractChildNodes,
							CostConfig: abstractCostConfig,
						},
						abstractCustomConfig,
					),
				},
				fields: abstractFieldConfig,
			},
			"external: field 'Paginated.items' requires exactly one slicing argument, but 2 were provided, locations: [], path: [search,items]",
			computeCosts(),
		))

	})
}
