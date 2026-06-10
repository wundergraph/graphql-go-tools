package engine

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

func TestExecutionEngine_Cost(t *testing.T) {
	t.Parallel()
	t.Run("common on star wars scheme", func(t *testing.T) {
		t.Parallel()
		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"hero", "droid", "search", "searchResults"}},
			{TypeName: "Human", FieldNames: []string{"name", "height", "friends"}},
			{TypeName: "Droid", FieldNames: []string{"name", "primaryFunction", "friends"}},
			{TypeName: "Starship", FieldNames: []string{"name", "length"}},
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(18), // Query.droid (1) + droid.name (17)
				expectedActualCost:    intPtr(18),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(21), // Query.droid (1) + Query.droid.id (3) + droid.name (17)
				expectedActualCost:    intPtr(21),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(0),
				expectedActualCost:    intPtr(0),
			},
			computeCosts(),
		))

		t.Run("hero field returning interface with concrete fragment", runWithoutError(
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
							Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(22), // Query.hero (2) + Human.height (3) + Droid.name (17=max(7, 17))
				expectedActualCost:    intPtr(22),
			},
			computeCosts(),
		))

		t.Run("hero field returning interface with concrete fragment with no matching typename", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								hero { 
									... on Droid { name }
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
							Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedResponse:      `{"data":{"hero":{}}}`,
				expectedEstimatedCost: intPtr(19), // Query.hero (2) + Droid.name (17=max(7, 17))
				expectedActualCost:    intPtr(2),
			},
			computeCosts(),
		))

		// Regression test for the abstract field without __typename bug recordObjectTypeStats).
		// When the subgraph resolves a single (non-list) abstract field and does NOT return __typename,
		// we must still record one occurrence for that field's path, falling back to the declared
		// abstract type name in actual costs.
		t.Run("single abstract field without __typename takes into account implementing types", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ hero { ... on Human { height } } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// No __typename returned for the abstract hero field.
								sendResponseBody: `{"data":{"hero":{"height":"12"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
							Weights: map[plan.FieldCoordinate]*plan.FieldCost{},
							Types: map[string]int{
								"Human": 13,
								"Droid": 7,
							},
						}},
						customConfig,
					),
				},
				expectedEstimatedCost: intPtr(13), // Query.hero(13)
				expectedActualCost:    intPtr(13), // Query.hero(13)
			},
			computeCosts(),
		))

		t.Run("field returning a list of abstracts with partial fragments considers only actual type counts", runWithoutError(
			ExecutionEngineTestCase{
				schema: graphql.StarwarsSchema(t),
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ 
								searchResults { 
									... on Droid { name }
									... on Human { name height }
								}
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"searchResults":[{"__typename":"Human","name":"Luke","height":"12"},{"__typename":"Droid","name":"D2","primaryFunction":"charge"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: &plan.DataSourceCostConfig{
							Weights: map[plan.FieldCoordinate]*plan.FieldCost{
								{TypeName: "Query", FieldName: "searchResults"}: {HasWeight: true, Weight: 2},
								{TypeName: "Human", FieldName: "height"}:        {HasWeight: true, Weight: 3},
								{TypeName: "Human", FieldName: "name"}:          {HasWeight: true, Weight: 7},
								{TypeName: "Droid", FieldName: "name"}:          {HasWeight: true, Weight: 17},
							},
							Types: map[string]int{
								"Human": 13,
							},
						}},
						customConfig,
					),
				},
				expectedResponse:      `{"data":{"searchResults":[{"name":"Luke","height":"12"},{"name":"D2"}]}}`,
				expectedEstimatedCost: intPtr(190), // 190 = 10 * (2+max(17, 7+3))
				expectedActualCost:    intPtr(31),  // 31 = 2 * (2) + 1 * (17) + 1 * (7+3)
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
							Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(30), // Query.Human (13) + Droid.name (17=max(7, 17))
				// name is interface so the actual cost is taken as max
				expectedActualCost: intPtr(30),
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
								sendResponseBody: `{"data":{"hero":{"__typename":"Droid","friends":[
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(107), // 7 + 10*(max(7,5) + max(Human(2+1),Droid(2)))
				expectedActualCost:    intPtr(22),  // 5 +  2*(       6 + 0.5 * (2+0+2+1))
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(207), // max(7,5)+ 20 * (7 + max(2,2+1))
				expectedActualCost:    intPtr(24),  // hero(7) +  2 * (6 + 0.5*(2+0+2+1))
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(147), // hero(max(7,5))+ 20 * (4+max(2, 2+1))
				expectedActualCost:    intPtr(20),  // hero(7)       +  2 * (4+0.5*(2+2+1))
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
				expectedEstimatedCost: intPtr(11), // Query.hero(max(1,1))+ 10 * 1
				expectedActualCost:    intPtr(3),  // Query.hero(1) + 2 * 1
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				// Estimated with default list size 10: hero(7) + 10 * (7 + max(2, 2+1))
				expectedEstimatedCost: intPtr(107),
				// Actual uses real list size 2:        hero(7) +  2 * (6 + 0.5 * (2 + 2 + 1))
				expectedActualCost: intPtr(24),
			},
			computeCosts(),
		))

		t.Run("actual cost with empty nested list", runWithoutError(
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				// default list size 10: hero(7) + 10 * (max(7,5) + max(2, 2))
				expectedEstimatedCost: intPtr(97),
				// empty list returned: hero(7) + 0 * (7 + 2 + 2)
				expectedActualCost: intPtr(7),
			},
			computeCosts(),
		))

		t.Run("named fragment on interface without typenames on friends", runWithoutError(
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
								expectedHost: "example.com",
								expectedPath: "/",
								expectedBody: "",
								// Is it possible that friends items would be returned without __typename?
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","friends":[{"name":"Leia"}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(55), // 2 + 1*(5 + 6*(3 + 1*5))
				expectedActualCost:    intPtr(15), // 2 + 1*(5 + 1*(3 + 1*5))
			},
			computeCosts(),
		))

		t.Run("named fragment on interface with typename on friends", runWithoutError(
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
								sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke","friends":[{"__typename":"Human","name":"Leia"}]}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(55),
				expectedActualCost:    intPtr(12), // 2 + 1*5 + 1*(2 + 1*3)
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(12),
				expectedActualCost:    intPtr(12),
			},
			computeCosts(),
		))

		t.Run("cost on argument of directive", func(t *testing.T) {
			t.Run("directive with default non-null argument on a field adds to cost", runWithoutError(
				// search(name: String!): SearchResult @approx
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
								search(name: "Luke") {
									... on Human { name }
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
									sendResponseBody: `{"data":{"search":{"__typename":"Human","name":"Luke"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Query", FieldName: "search"}: {
											HasWeight:                true,
											Weight:                   3,
											ArgumentWeights:          map[string]int{"name": 2},
											DirectiveArgumentWeights: map[string]int{"approx.tolerance": -5},
										},
										{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 5},
									},
								},
							},
							customConfig,
						),
					},
					fields: []plan.FieldConfiguration{
						{
							TypeName: "Query", FieldName: "search",
							Arguments: []plan.ArgumentConfiguration{
								{
									Name:         "name",
									SourceType:   plan.FieldArgumentSource,
									RenderConfig: plan.RenderArgumentAsGraphQLValue,
								},
							},
						},
					},
					expectedResponse: `{"data":{"search":{"name":"Luke"}}}`,
					// Query.search(3) + name arg(2) + Human.name(5) + @approx.tolerance(-5) = 5
					expectedEstimatedCost: intPtr(5),
					expectedActualCost:    intPtr(5),
				},
				computeCosts(),
			))

			t.Run("querying interface accounts for directive costs on implementations", runWithoutError(
				// type Droid implements Character { name: String! @approx }
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
									expectedHost:     "example.com",
									expectedPath:     "/",
									expectedBody:     "",
									sendResponseBody: `{"data":{"hero":{"__typename":"Human","name":"Luke Skywalker"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Human", FieldName: "name"}: {HasWeight: true, Weight: 5},
										{TypeName: "Droid", FieldName: "name"}: {DirectiveArgumentWeights: map[string]int{"approx.tolerance": -5}},
									},
								},
							},
							customConfig,
						),
					},
					fields:           []plan.FieldConfiguration{},
					expectedResponse: `{"data":{"hero":{"name":"Luke Skywalker"}}}`,
					// Query.hero(1) + Human.name(5) + @approx.tolerance(-5) = 1
					expectedEstimatedCost: intPtr(1),
					expectedActualCost:    intPtr(1),
				},
				computeCosts(),
			))

			t.Run("field with directive of null-value arg does not affect cost", runWithoutError(
				// droid(id: ID!): Droid @approx(tolerance: null)
				ExecutionEngineTestCase{
					schema: graphql.StarwarsSchema(t),
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
								droid(id: "R2D2") {
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
									sendResponseBody: `{"data":{"droid":{"primaryFunction":"no"}}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Droid", FieldName: "primaryFunction"}: {HasWeight: true, Weight: 17},
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
					expectedResponse: `{"data":{"droid":{"primaryFunction":"no"}}}`,
					// Query.droid (1) + droid.primaryFunction (17); @approx.tolerance is null
					expectedEstimatedCost: intPtr(18),
					expectedActualCost:    intPtr(18),
				},
				computeCosts(),
			))
		})

	})

	t.Run("union types", func(t *testing.T) {
		t.Parallel()
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
							RootNodes: rootNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				//   Type weight: max(User=5, Post=9, Comment=1) = 9
				expectedEstimatedCost: intPtr(60), // 5 * (3 + max(5, 9, 1))
				expectedActualCost:    intPtr(7),  // 1 * (2 + 1*2 + 1*3)
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
							RootNodes: rootNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"name":"John"}]}}`,
				expectedEstimatedCost: intPtr(45), // 3 * (max(6,10) + max(2,5))
				expectedActualCost:    intPtr(8),  // 1 * (6 + 1*2)
			},
			computeCosts(),
		))
	})

	t.Run("listSize", func(t *testing.T) {
		t.Parallel()
		listSchema := `
			input Search {
                pagination: Page
                query: String
			}
			input Page {
                first: Int
			}
			type Query {
			    items(first: Int, last: Int): [Item!]
			    search(input: Search): [Item!]
			}
			type Item @key(fields: "id") {
			    id: ID
			}
			`
		schemaSlicing, err := graphql.NewSchemaFromString(listSchema)
		require.NoError(t, err)
		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"items"}},
			{TypeName: "Query", FieldNames: []string{"search"}},
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
				FieldName: "search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{
						Name:         "input",
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(48), // slicingArgument(12) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(100), // slicingArgument($limit=25) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),   // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("dot-path slicing argument passed as literal is valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput{
							  search(input: { pagination: { first: 8 }, query: "abc" }) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										RequireOneSlicingArgument: true,
										SlicingArguments:          []string{"input.pagination.first"},
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(32), // slicingArgument(8) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("slicing argument as nested input literal missing leaf fallbacks to defaultListSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput{
							  search(input: { pagination: { first: null }, query: "abc" }) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										RequireOneSlicingArgument: false,
										SlicingArguments:          []string{"input.pagination.first"},
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(40), // defaultListSize(10) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("slicing argument as nested input literal with null at the middle fallbacks to AssumedSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput{
							  search(input: { pagination: null, query: "abc" }) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										RequireOneSlicingArgument: false,
										AssumedSize:               15,
										SlicingArguments:          []string{"input.pagination.first"},
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(60), // AssumedSize(15) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("slicing argument as nested input literal starting with null fallbacks to defaultListSize", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput{
							  search(input: null) { id }
							}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										RequireOneSlicingArgument: false,
										SlicingArguments:          []string{"input.pagination.first"},
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(40), // defaultListSize(10) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("slicing argument as nested input variable is valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput($input: Search) {
							  search(input: $input) { id }
							}`,
						Variables: []byte(`{"input":{"pagination":{"first":12},"query":"abc"}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										SlicingArguments: []string{"input.pagination.first"},
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(48), // slicingArgument($input.pagination.first=12) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("required dot-path slicing argument passed as var is valid", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput($input: Search) {
							  search(input: $input) { id }
							}`,
						Variables: []byte(`{"input":{"pagination":{"first":7},"query":"abc"}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[ {"id":"2"}, {"id":"3"} ]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										SlicingArguments:          []string{"input.pagination.first"},
										RequireOneSlicingArgument: true,
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"search":[{"id":"2"},{"id":"3"}]}}`,
				expectedEstimatedCost: intPtr(28), // slicingArgument(7) * (Item(3)+Item.id(1))
				expectedActualCost:    intPtr(8),  // 2 * (Item(3)+Item.id(1))
			},
			computeCosts(),
		))

		t.Run("required dot-path slicing argument missing intermediate is invalid", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput($input: Search) {
							  search(input: $input) { id }
							}`,
						Variables: []byte(`{"input":{"query":"abc"}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										SlicingArguments:          []string{"input.pagination.first"},
										RequireOneSlicingArgument: true,
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields: fieldConfig,
			},
			"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
			computeCosts(),
		))

		t.Run("required dot-path slicing argument missing leaf is invalid", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput($input: Search) {
							  search(input: $input) { id }
							}`,
						Variables: []byte(`{"input":{"pagination":{"first":null},"query":"abc"}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										SlicingArguments:          []string{"input.pagination.first"},
										RequireOneSlicingArgument: true,
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields: fieldConfig,
			},
			"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
			computeCosts(),
		))

		t.Run("required dot-path slicing argument with empty variables is invalid", runWithAndCompareError(
			ExecutionEngineTestCase{
				schema: schemaSlicing,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `query NestedInput($input: Search) {
							  search(input: $input) { id }
							}`,
						Variables: []byte(`{}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"search":[]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{
							RootNodes:  rootNodes,
							ChildNodes: childNodes,
							CostConfig: &plan.DataSourceCostConfig{
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
									{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
								},
								ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
									{TypeName: "Query", FieldName: "search"}: {
										SlicingArguments:          []string{"input.pagination.first"},
										RequireOneSlicingArgument: true,
									},
								},
								Types: map[string]int{"Item": 3},
							},
						},
						customConfig,
					),
				},
				fields: fieldConfig,
			},
			"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(45), // Total: 15 * (2 + 1)
				expectedActualCost:    intPtr(6),  // 2 * (2 + 1)
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(60), // 20 * (2 + 1)
				expectedActualCost:    intPtr(0),  // empty response list
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(75), //  25 * (2 + 1)
				expectedActualCost:    intPtr(0),  // empty response list
			},
			computeCosts(),
		))

		t.Run("sliceArguments with defaulted arguments", func(t *testing.T) {
			// When a slicing argument is omitted from the operation,
			// the engine must fall back to the upstream SDL default.
			listSchemaWithDefaults := `
				input Search {
					pagination: Page
					query: String
				}
				input Page {
					first: Int = 10
				}
				type Query {
					items(first: Int = 25, last: Int = 10): [Item!]
					search(input: Search = { pagination: { first: 8 } }): [Item!]
				}
				type Item @key(fields: "id") {
					id: ID
				}
			`
			schemaSlicingDefaults, err := graphql.NewSchemaFromString(listSchemaWithDefaults)
			require.NoError(t, err)
			customConfigDefaults := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, listSchemaWithDefaults),
			})

			t.Run("flat slicing arg omitted - uses schema Int default", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicingDefaults,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query FlatDefault { items { id } }`,
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
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "items"}: {
											AssumedSize:      8,
											SlicingArguments: []string{"first", "last"},
										},
									},
									Types: map[string]int{"Item": 3},
								},
							},
							customConfigDefaults,
						),
					},
					fields:                fieldConfig,
					expectedResponse:      `{"data":{"items":[{"id":"2"},{"id":"3"}]}}`,
					expectedEstimatedCost: intPtr(100), // max(first=25, last=10) * (Item(3)+Item.id(1))
					expectedActualCost:    intPtr(8),   // 2 * (Item(3)+Item.id(1))
				},
				computeCosts(),
			))

			t.Run("when dot-path arg omitted, outer object-literal default supplies leaf", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicingDefaults,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query OuterObjectDefault { search { id } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"search":[ {"id":"2"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {
											RequireOneSlicingArgument: true,
											SlicingArguments:          []string{"input.pagination.first"},
										},
									},
									Types: map[string]int{"Item": 3},
								},
							},
							customConfigDefaults,
						),
					},
					fields:                fieldConfig,
					expectedResponse:      `{"data":{"search":[{"id":"2"}]}}`,
					expectedEstimatedCost: intPtr(32), // outer default { pagination: { first: 8 } } * (Item(3)+Item.id(1))
					expectedActualCost:    intPtr(4),  // 1 * (Item(3)+Item.id(1))
				},
				computeCosts(),
			))

			t.Run("when dot-path with partially provided input, inner field default supplies leaf", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicingDefaults,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							// `pagination` is provided as an empty object — `first` is absent
							// and must resolve to the Page.first schema default (= 10).
							Query: `query InnerFieldDefault { search(input: { pagination: {}, query: "q" }) { id } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"search":[ {"id":"2"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {
											RequireOneSlicingArgument: true,
											SlicingArguments:          []string{"input.pagination.first"},
										},
									},
									Types: map[string]int{"Item": 3},
								},
							},
							customConfigDefaults,
						),
					},
					fields:                fieldConfig,
					expectedResponse:      `{"data":{"search":[{"id":"2"}]}}`,
					expectedEstimatedCost: intPtr(40), // inner Page.first default (10) * (Item(3)+Item.id(1))
					expectedActualCost:    intPtr(4),  // 1 * (Item(3)+Item.id(1))
				},
				computeCosts(),
			))

			t.Run("explicit null at dot-path leaf must not use schema default", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicingDefaults,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query ExplicitNullLeaf { search(input: { pagination: { first: null }, query: "q" }) { id } }`,
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"search":[ {"id":"2"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {
											AssumedSize:      5,
											SlicingArguments: []string{"input.pagination.first"},
										},
									},
									Types: map[string]int{"Item": 3},
								},
							},
							customConfigDefaults,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"search":[{"id":"2"}]}}`,
					// AssumedSize (5) * (Item(3)+Item.id(1))
					expectedEstimatedCost: intPtr(20),
					// 1 * (Item(3)+Item.id(1))
					expectedActualCost: intPtr(4),
				},
				computeCosts(),
			))

			t.Run("variable-nulled dot-path leaf must not use schema default", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaSlicingDefaults,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query VarNullLeaf($input: Search) { search(input: $input) { id } }`,
							Variables: []byte(`{"input":{"pagination":{"first":null},"query":"q"}}`),
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"search":[ {"id":"2"} ]}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  rootNodes,
								ChildNodes: childNodes,
								CostConfig: &plan.DataSourceCostConfig{
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
										{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
									},
									ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
										{TypeName: "Query", FieldName: "search"}: {
											AssumedSize:      5,
											SlicingArguments: []string{"input.pagination.first"},
										},
									},
									Types: map[string]int{"Item": 3},
								},
							},
							customConfigDefaults,
						),
					},
					fields:           fieldConfig,
					expectedResponse: `{"data":{"search":[{"id":"2"}]}}`,
					// AssumedSize (5) * (Item(3)+Item.id(1))
					expectedEstimatedCost: intPtr(20),
					// 1 * (Item(3)+Item.id(1))
					expectedActualCost: intPtr(4),
				},
				computeCosts(),
			))
		})

	})

	t.Run("nested lists with compounding multipliers", func(t *testing.T) {
		t.Parallel()
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(640),
				expectedActualCost:    intPtr(10),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(1508),
				expectedActualCost:    intPtr(10),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(640),
				// Actual cost with 1 item at each level:
				// Total: 1 * (4 + 1 * (3 + 1 * (2 + 1))) = 10
				expectedActualCost: intPtr(10),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(640),
				// Actual cost: 2 * (4 + 2 * (3 + 3 * (2 + 1))) = 56
				expectedActualCost: intPtr(56),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(490),
				// Actual cost: 2 * (4 + 1.5 * (3 + 3 * (2 + 1))) = 44
				expectedActualCost: intPtr(44),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(50),
				// Actual cost: 3 users at root
				// 3 * (4 + 1) = 15
				expectedActualCost: intPtr(15),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(640), // 10 * (4 + 5 * (3 + 3 * (2 + 1)))
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
				expectedActualCost: intPtr(33),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(211110),
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
				expectedActualCost: intPtr(51),
			},
			computeCosts(),
		))
	})

	t.Run("abstract list with concrete fragment charges only matched items", func(t *testing.T) {
		t.Parallel()
		schemaStr := `
			type Query {
			  boards(ids: [ID!]!, limit: Int): [Board!]!
				  # @listSize(slicingArguments: ["limit"]) 
				  # @cost(weight: 10)
			}
			type Board {
			  items_page(limit: Int!): ItemsPage!
				  # @listSize(slicingArguments: ["limit"], sizedFields: ["items"]) 
				  # @cost(weight: 10)
			}
			type ItemsPage {
			  items: [Item!]!   # @cost(weight: 10)
			}
			type Item {
			  column_values: [ColumnValue!]!
			}
			interface ColumnValue {
			  text: String
			}
			type PeopleValue implements ColumnValue {
			  text: String    # @cost(weight: 10)
			  person: Person  
			}
			type StatusValue implements ColumnValue {
			  text: String
			}
			type Person {
			  name: String    # @cost(weight: 10)
			} `
		schemaAbs, err := graphql.NewSchemaFromString(schemaStr)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"boards"}},
			{TypeName: "Board", FieldNames: []string{"items_page"}},
			{TypeName: "ItemsPage", FieldNames: []string{"items"}},
			{TypeName: "Item", FieldNames: []string{"column_values"}},
			{TypeName: "PeopleValue", FieldNames: []string{"text", "person"}},
			{TypeName: "StatusValue", FieldNames: []string{"text"}},
			{TypeName: "Person", FieldNames: []string{"name"}},
		}
		childNodes := []plan.TypeField{
			{TypeName: "ColumnValue", FieldNames: []string{"text"}},
		}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, schemaStr),
		})
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName: "Query", FieldName: "boards", Path: []string{"boards"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "ids", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "limit", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Board", FieldName: "items_page", Path: []string{"items_page"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "limit", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		costConfig := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
				{TypeName: "Query", FieldName: "boards"}:     {HasWeight: true, Weight: 10},
				{TypeName: "Board", FieldName: "items_page"}: {HasWeight: true, Weight: 10},
				{TypeName: "ItemsPage", FieldName: "items"}:  {HasWeight: true, Weight: 10},
				{TypeName: "PeopleValue", FieldName: "text"}: {HasWeight: true, Weight: 10},
				{TypeName: "Person", FieldName: "name"}:      {HasWeight: true, Weight: 10},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "boards"}: {
					SlicingArguments: []string{"limit"},
				},
				{TypeName: "Board", FieldName: "items_page"}: {
					SlicingArguments: []string{"limit"},
					SizedFields:      []string{"items"},
				},
			},
		}

		query := `{
			boards(ids: ["b1"], limit: 1) {
				items_page(limit: 1) {
					items {
						column_values {
							... on PeopleValue { text }
						}
					}
				}
			}
		}`

		t.Run("no matching items: text contributes 0", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 3 column_values (0 PeopleValue).
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{},{},{}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*10))))
				expectedEstimatedCost: intPtr(140),
				// 1 * (10 + 1 * (10 + 1 * (10 + 1 * (1 + 0*10))))
				expectedActualCost: intPtr(33),
			},
			computeCosts(),
		))

		t.Run("partial match: text charged per PeopleValue only", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 10 column_values, 4 PeopleValue
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1"},` +
									`{"__typename":"PeopleValue","text":"p2"},` +
									`{"__typename":"PeopleValue","text":"p3"},` +
									`{"__typename":"PeopleValue","text":"p4"},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"},` +
									`{"__typename":"StatusValue","text":"s4"},` +
									`{"__typename":"StatusValue","text":"s5"},` +
									`{"__typename":"StatusValue","text":"s6"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields: fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
					`{"text":"p1"},{"text":"p2"},{"text":"p3"},{"text":"p4"},{},{},{},{},{},{}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*10))))
				expectedEstimatedCost: intPtr(140),
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 0.4*10))))
				expectedActualCost: intPtr(80),
			},
			computeCosts(),
		))

		t.Run("nested field charged per matched PeopleValue", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										... on PeopleValue { person { name } }
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 10 column_values, 4 PeopleValue with nested person
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","person":{"name":"p1"}},` +
									`{"__typename":"PeopleValue","person":{"name":"p2"}},` +
									`{"__typename":"PeopleValue","person":{"name":"p3"}},` +
									`{"__typename":"PeopleValue","person":{"name":"p4"}},` +
									`{"__typename":"StatusValue"},` +
									`{"__typename":"StatusValue"},` +
									`{"__typename":"StatusValue"},` +
									`{"__typename":"StatusValue"},` +
									`{"__typename":"StatusValue"},` +
									`{"__typename":"StatusValue"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields: fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
					`{"person":{"name":"p1"}},{"person":{"name":"p2"}},{"person":{"name":"p3"}},{"person":{"name":"p4"}},` +
					`{},{},{},{},{},{}]}]}}]}}`,
				// Estimation (default list size 10): abstract picks the max-cost member subtree.
				//   PeopleValue member subtree = person(1) + name(10) = 11
				//   1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 11))))
				expectedEstimatedCost: intPtr(150),
				// Actual: 10 column_values, 4 matched PeopleValue.
				//   name multiplier = 1 (one Person per matched item), person multiplier = 4/10.
				//   1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 0.4 * (1 + 10)))))
				expectedActualCost: intPtr(84),
			},
			computeCosts(),
		))

		t.Run("nested field charged per matched PeopleValue once for duplicates", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										text
										... on PeopleValue { text person { name } }
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 10 column_values, 4 PeopleValue with nested person
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1","person":{"name":"p1"}},` +
									`{"__typename":"PeopleValue","text":"p2","person":{"name":"p2"}},` +
									`{"__typename":"PeopleValue","text":"p3","person":{"name":"p3"}},` +
									`{"__typename":"PeopleValue","text":"p4","person":{"name":"p4"}},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"},` +
									`{"__typename":"StatusValue","text":"s4"},` +
									`{"__typename":"StatusValue","text":"s5"},` +
									`{"__typename":"StatusValue","text":"s6"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{"text":"p1","person":{"name":"p1"}},{"text":"p2","person":{"name":"p2"}},{"text":"p3","person":{"name":"p3"}},{"text":"p4","person":{"name":"p4"}},{"text":"s1"},{"text":"s2"},{"text":"s3"},{"text":"s4"},{"text":"s5"},{"text":"s6"}]}]}}]}}`,
				//   1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 10 + 1 + 10))))
				expectedEstimatedCost: intPtr(250),
				// Actual: 10 column_values, 4 matched PeopleValue.
				//   name multiplier = 1 (one Person per matched item), person multiplier = 4/10.
				//   1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 0.4*10 + 0.4 * (1 + 10)))))
				expectedActualCost: intPtr(124),
			},
			computeCosts(),
		))

		t.Run("a field selected directly on the interface should be charged only for implementing fields with weights", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										text
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1"},` +
									`{"__typename":"PeopleValue","text":"p2"},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{"text":"p1"},{"text":"p2"},{"text":"s1"},{"text":"s2"},{"text":"s3"}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*10))))
				expectedEstimatedCost: intPtr(140),
				// 1 * (10 + 1 * (10 + 1 * (10 + 5 * (1 + 0.4*10))))
				expectedActualCost: intPtr(55),
			},
			computeCosts(),
		))

		t.Run("a field selected on the interface and fragments should be charged once", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										text
										... on PeopleValue { text }
										... on StatusValue { text }
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1"},` +
									`{"__typename":"PeopleValue","text":"p2"},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{"text":"p1"},{"text":"p2"},{"text":"s1"},{"text":"s2"},{"text":"s3"}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*10))))
				expectedEstimatedCost: intPtr(140),
				// 1 * (10 + 1 * (10 + 1 * (10 + 5 * (1 + 0.4*10))))
				expectedActualCost: intPtr(55),
			},
			computeCosts(),
		))

		t.Run("a field selected on fragments should be charged once", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										... on PeopleValue { text }
										... on StatusValue { text }
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1"},` +
									`{"__typename":"PeopleValue","text":"p2"},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{"text":"p1"},{"text":"p2"},{"text":"s1"},{"text":"s2"},{"text":"s3"}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*10))))
				expectedEstimatedCost: intPtr(140),
				// 1 * (10 + 1 * (10 + 1 * (10 + 5 * (1 + 0.4*10))))
				expectedActualCost: intPtr(55),
			},
			computeCosts(),
		))

		// An explicit `HasWeight: true, Weight: 0` on a per-type fragment field must be
		// respected; the cost calculator must NOT fall back to type-default weights.
		zeroWeightCostConfig := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
				{TypeName: "Query", FieldName: "boards"}:     {HasWeight: true, Weight: 10},
				{TypeName: "Board", FieldName: "items_page"}: {HasWeight: true, Weight: 10},
				{TypeName: "ItemsPage", FieldName: "items"}:  {HasWeight: true, Weight: 10},
				{TypeName: "PeopleValue", FieldName: "text"}: {HasWeight: true, Weight: 0}, // explicit zero
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "boards"}: {
					SlicingArguments: []string{"limit"},
				},
				{TypeName: "Board", FieldName: "items_page"}: {
					SlicingArguments: []string{"limit"},
					SizedFields:      []string{"items"},
				},
			},
			Types: map[string]int{"String": 5},
		}

		t.Run("explicit Weight: 0 on a fragment field stays zero", runWithoutError(
			ExecutionEngineTestCase{
				schema: schemaAbs,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: `{
						boards(ids: ["b1"], limit: 1) {
							items_page(limit: 1) {
								items {
									column_values {
										... on PeopleValue { text }
									}
								}
							}
						}}`}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 5 column_values: 2 PeopleValue + 3 StatusValue. text present on all.
								sendResponseBody: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[` +
									`{"__typename":"PeopleValue","text":"p1"},` +
									`{"__typename":"PeopleValue","text":"p2"},` +
									`{"__typename":"StatusValue","text":"s1"},` +
									`{"__typename":"StatusValue","text":"s2"},` +
									`{"__typename":"StatusValue","text":"s3"}` +
									`]}]}}]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: zeroWeightCostConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"boards":[{"items_page":{"items":[{"column_values":[{"text":"p1"},{"text":"p2"},{},{},{}]}]}}]}}`,
				// 1 * (10 + 1 * (10 + 1 * (10 + 10 * (1 + 1*0))))
				expectedEstimatedCost: intPtr(40), //
				// 1 * (10 + 1 * (10 + 1 * (10 + 5 * (1 + 0*0.4))))
				expectedActualCost: intPtr(35),
			},
			computeCosts(),
		))
	})

	t.Run("nested abstract lists", func(t *testing.T) {
		t.Parallel()
		schemaStr := `
			type Query {
			  things: [Thing!]!
			}
			interface Thing {
			  id: ID
			}
			type Foo implements Thing {
			  id: ID
			  related: [Related!]!
			}
			type Bar implements Thing {
			  id: ID
			}
			interface Related {
			  code: ID
			}
			type Alpha implements Related {
			  code: ID
			  name: String
			}
			type Beta implements Related {
			  code: ID
			} `
		schema, err := graphql.NewSchemaFromString(schemaStr)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"things"}},
			{TypeName: "Foo", FieldNames: []string{"id", "related"}},
			{TypeName: "Bar", FieldNames: []string{"id"}},
			{TypeName: "Alpha", FieldNames: []string{"code", "name"}},
			{TypeName: "Beta", FieldNames: []string{"code"}},
		}
		childNodes := []plan.TypeField{
			{TypeName: "Thing", FieldNames: []string{"id"}},
			{TypeName: "Related", FieldNames: []string{"code"}},
		}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, schemaStr),
		})

		costConfig := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
				{TypeName: "Query", FieldName: "things"}: {HasWeight: true, Weight: 10},
				{TypeName: "Foo", FieldName: "related"}:  {HasWeight: true, Weight: 5},
				{TypeName: "Alpha", FieldName: "name"}:   {HasWeight: true, Weight: 20},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "things"}: {AssumedSize: 8},
				{TypeName: "Foo", FieldName: "related"}:  {AssumedSize: 7},
			},
		}

		query := `{ things {
			... on Bar { id }
			... on Foo {
				related {
					... on Alpha { name } } } } }`

		t.Run("name charged per matched Alpha across all Foos", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 4 things: 2 Foo + 2 Bar. Each Foo's related: 3 items.
								// All Foos have 3 Alpha + 3 Beta.
								sendResponseBody: `{"data":{"things":[` +
									`{"__typename":"Foo","related":[` +
									`{"__typename":"Alpha","name":"a1"},` +
									`{"__typename":"Alpha","name":"a2"},` +
									`{"__typename":"Beta"}` +
									`]},` +
									`{"__typename":"Foo","related":[` +
									`{"__typename":"Alpha","name":"a3"},` +
									`{"__typename":"Beta"},` +
									`{"__typename":"Beta"}` +
									`]},` +
									`{"__typename":"Bar","id":"1"},` +
									`{"__typename":"Bar","id":"2"}` +
									`]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"things":[` +
					`{"related":[{"name":"a1"},{"name":"a2"},{}]},` +
					`{"related":[{"name":"a3"},{},{}]},` +
					`{"id":"1"},{"id":"2"}` +
					`]}}`,
				// 8 * (10 + 1   * 0 + 7   * (5 + 1*20))
				expectedEstimatedCost: intPtr(1480),
				// 4 * (10 + 0.5 * 0 + 1.5 * (5 + 0.5*20))
				expectedActualCost: intPtr(130),
			},
			computeCosts(),
		))
		t.Run("name is not charged when related's alphas are empty", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 4 things: 2 Foo + 2 Bar. Related are empty.
								sendResponseBody: `{"data":{"things":[` +
									`{"__typename":"Foo","related":[` +
									`{"__typename":"Beta"}` +
									`]},` +
									`{"__typename":"Foo","related":[` +
									`{"__typename":"Beta"},` +
									`{"__typename":"Beta"}` +
									`]},` +
									`{"__typename":"Bar","id":"1"},` +
									`{"__typename":"Bar","id":"2"}` +
									`]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"things":[` +
					`{"related":[{}]},` +
					`{"related":[{},{}]},` +
					`{"id":"1"},{"id":"2"}` +
					`]}}`,
				// 8 * (10 + 1   * 0 + 7    * (5 + 1*20))
				expectedEstimatedCost: intPtr(1480),
				// 4 * (10 + 0.5 * 0 + 0.75 * (5 + 0*20)))
				expectedActualCost: intPtr(55),
			},
			computeCosts(),
		))
		t.Run("Alpha is not charged at all", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 2 things: 2 Bar.
								sendResponseBody: `{"data":{"things":[` +
									`{"__typename":"Bar","id":"1"},` +
									`{"__typename":"Bar","id":"2"}` +
									`]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"things":[` +
					`{"id":"1"},{"id":"2"}` +
					`]}}`,
				// 8 * (10 + 1 * 0 + 7 * (5 + 1*20))
				expectedEstimatedCost: intPtr(1480),
				// 2 * (10 + 1 * 0 + 0 * (5 + 1*20))
				expectedActualCost: intPtr(20),
			},
			computeCosts(),
		))
		t.Run("empty response returns zero cost", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{Query: query}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								// 2 things: 2 Bar.
								sendResponseBody: `{"data":{"things":[` +
									`]}}`,
								sendStatusCode: 200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, ChildNodes: childNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				expectedResponse: `{"data":{"things":[` +
					`]}}`,
				// 8 * (10 + 1 * 0 + 7 * (5 + 1*20))
				expectedEstimatedCost: intPtr(1480),
				// 0 * (10 + 0 * 0 + 0 * (5 + 1*20))
				expectedActualCost: intPtr(0),
			},
			computeCosts(),
		))
	})

	t.Run("sizedFields", func(t *testing.T) {
		t.Parallel()
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(57),
				// UserConnection(1) + Int(0) + 1*(UserEdge(1)+User(1)+User.name(2)) + 1*(User(1)+User.name(2))
				expectedActualCost: intPtr(8),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(13),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: intPtr(5),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(61),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2)+1*(Post(1)+Post.title(3)))
				expectedActualCost: intPtr(9),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(41),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: intPtr(5),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(1),
				expectedActualCost:    intPtr(1),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(29),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: intPtr(5),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(41),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2))
				expectedActualCost: intPtr(5),
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
								Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(51),
				// UserConnection(1) + 1*(UserEdge(1)+User(1)+User.name(2)) + 1*(User(1)+User.name(2))
				expectedActualCost: intPtr(8),
			},
			computeCosts(),
		))

		t.Run("sizedFields parent is non-list wrapper inside outer list", func(t *testing.T) {
			// Regression test for ENG-9574.
			// When a non-list wrapper field (Board.items_page: ItemsPage!) is configured as a
			// @listSize sizedFields parent, the wrapper itself is never recorded in
			// typeNameStats (population happens in walkArray only). The child list's
			// averaging denominator therefore falls back to 1 instead of the number of
			// wrapper occurrences, inflating the combined actual cost.
			boardsSchema := `
				type Query {
					boards(ids: [ID!]!, limit: Int): [Board!]! 
						@listSize(slicingArguments: ["limit"]) 
						@cost(weight: 10)
				}
				type Board @key(fields: "id") {
					id: ID!
					items_page(limit: Int!): ItemsPage! 
						@listSize(slicingArguments: ["limit"], sizedFields: ["items"]) 
						@cost(weight: 10)
				}
				type ItemsPage {
					items: [Item!]! 
						@cost(weight: 10)
				}
				type Item @key(fields: "id") {
					id: ID!
				}
			`
			schemaBoards, err := graphql.NewSchemaFromString(boardsSchema)
			require.NoError(t, err)

			boardsRootNodes := []plan.TypeField{
				{TypeName: "Query", FieldNames: []string{"boards"}},
				{TypeName: "Board", FieldNames: []string{"id", "items_page"}},
				{TypeName: "ItemsPage", FieldNames: []string{"items"}},
				{TypeName: "Item", FieldNames: []string{"id"}},
			}
			boardsCustomConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
				Fetch: &graphql_datasource.FetchConfiguration{
					URL:    "https://example.com/",
					Method: "GET",
				},
				SchemaConfiguration: mustSchemaConfig(t, nil, boardsSchema),
			})
			boardsFieldConfig := []plan.FieldConfiguration{
				{
					TypeName: "Query", FieldName: "boards", Path: []string{"boards"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "ids", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
						{Name: "limit", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
				{
					TypeName: "Board", FieldName: "items_page", Path: []string{"items_page"},
					Arguments: []plan.ArgumentConfiguration{
						{Name: "limit", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					},
				},
			}
			boardsCostConfig := &plan.DataSourceCostConfig{
				Weights: map[plan.FieldCoordinate]*plan.FieldCost{
					{TypeName: "Query", FieldName: "boards"}:     {HasWeight: true, Weight: 10},
					{TypeName: "Board", FieldName: "items_page"}: {HasWeight: true, Weight: 10},
					{TypeName: "ItemsPage", FieldName: "items"}:  {HasWeight: true, Weight: 10},
				},
				ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
					{TypeName: "Query", FieldName: "boards"}: {
						SlicingArguments: []string{"limit"},
					},
					{TypeName: "Board", FieldName: "items_page"}: {
						SlicingArguments: []string{"limit"},
						SizedFields:      []string{"items"},
					},
				},
			}

			expectedResponse := `{"data":{"boards":[` +
				`{"id":"A","items_page":{"items":[{"id":"a1"}]}},` +
				`{"id":"B","items_page":{"items":[{"id":"b1"}]}},` +
				`{"id":"C","items_page":{"items":[{"id":"c1"}]}},` +
				`{"id":"D","items_page":{"items":[{"id":"d1"}]}}` +
				`]}}`

			// Correct behavior:
			//     parentCount should resolve to the nearest list ancestor,
			//     count (4 boards), giving items multiplier = 4/4 = 1.
			t.Run("actual cost averages by wrapper occurrences", runWithoutError(
				ExecutionEngineTestCase{
					schema: schemaBoards,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `{
								boards(ids: ["A","B","C","D"], limit: 4) {
									id
									items_page(limit: 1) {
										items { id }
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
									sendResponseBody: expectedResponse,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{
								RootNodes:  boardsRootNodes,
								ChildNodes: []plan.TypeField{},
								CostConfig: boardsCostConfig,
							},
							boardsCustomConfig,
						),
					},
					fields:           boardsFieldConfig,
					expectedResponse: expectedResponse,
					// 4 * ( 10 + 1 * (10 + 1 * 10))
					expectedEstimatedCost: intPtr(120),
					expectedActualCost:    intPtr(120),
				},
				computeCosts(),
			))
		})
	})

	t.Run("sizedFields on abstract types", func(t *testing.T) {
		t.Parallel()
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
									Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
					expectedEstimatedCost: intPtr(19),
					expectedActualCost:    intPtr(7),
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
					type UserConn implements Connection {
						edges: [Edge]
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
				{TypeName: "UserConn", FieldNames: []string{"edges"}},
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
									sendResponseBody: `{"data":{"users":{"__typename":"UserConn","edges":[{"cursor":"abc"}]}}}`,
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
					expectedEstimatedCost: intPtr(13),
					expectedActualCost:    intPtr(4),
				},
				computeCosts(),
			))
		})

		t.Run("sizedFields on interface field", func(t *testing.T) {
			t.Parallel()
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
					expectedEstimatedCost: intPtr(17),
					expectedActualCost:    intPtr(5),
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
					expectedEstimatedCost: intPtr(22),
					expectedActualCost:    intPtr(6),
				},
				computeCosts(),
			))
		})

		t.Run("sizedField returns list of abstract type", func(t *testing.T) {
			t.Parallel()
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
									sendResponseBody: `{"data":{"feed":{"items":[{"__typename":"Post","id":"1"},{"__typename":"Post","id":"2"}],"count":2}}}`,
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
					expectedEstimatedCost: intPtr(10),
					expectedActualCost:    intPtr(7),
				},
				computeCosts(),
			))
		})

	})

	t.Run("validate requireOneSlicingArgument on concrete types", func(t *testing.T) {
		t.Parallel()
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
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
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
				expectedEstimatedCost: intPtr(15), // assumedSize(5) * (Item(2) + Item.id(1))
				expectedActualCost:    intPtr(3),  // 1 * (Item(2) + Item.id(1))
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
				expectedEstimatedCost: intPtr(12), // 4 * (Item(2) + Item.id(1))
				expectedActualCost:    intPtr(3),  // 1 * (Item(2) + Item.id(1))
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
				expectedEstimatedCost: intPtr(30), // assumedSize(10) * (Item(2) + Item.id(1))
				expectedActualCost:    intPtr(3),  // 1 * (Item(2) + Item.id(1))
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
				expectedEstimatedCost: intPtr(15), // max(5,3)=5 * (Item(2) + Item.id(1))
				expectedActualCost:    intPtr(3),  // 1 * (Item(2) + Item.id(1))
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
				expectedEstimatedCost: intPtr(21), // 7 * (Item(2) + Item.id(1))
				expectedActualCost:    intPtr(3),  // 1 * (Item(2) + Item.id(1))
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

	t.Run("validate requireOneSlicingArgument with schema defaults", func(t *testing.T) {
		t.Parallel()
		listSchema := `
			input Page {
				first: Int = 8
			}
			type Query {
			   search(input: Page): [Item!]
			   items1(first: Int = 5, last: Int): [Item!]
			   items2(first: Int = 5, last: Int = 3): [Item!]
			}
			type Item @key(fields: "id") {
			  id: ID
			}
			`

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"items1", "items2", "search"}},
			{TypeName: "Item", FieldNames: []string{"id"}},
			{TypeName: "Page", FieldNames: []string{"first"}},
		}
		childNodes := []plan.TypeField{}

		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName:  "Query",
				FieldName: "items1",
				Path:      []string{"items1"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "items2",
				Path:      []string{"items2"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "first", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
					{Name: "last", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName:  "Query",
				FieldName: "search",
				Path:      []string{"search"},
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		costConfig := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
				{TypeName: "Item", FieldName: "id"}: {HasWeight: true, Weight: 1},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "items1"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: true,
				},
				{TypeName: "Query", FieldName: "items2"}: {
					AssumedSize:               10,
					SlicingArguments:          []string{"first", "last"},
					RequireOneSlicingArgument: true,
				},
				{TypeName: "Query", FieldName: "search"}: {
					SlicingArguments:          []string{"input.first"},
					RequireOneSlicingArgument: true,
				},
			},
			Types: map[string]int{"Item": 2},
		}
		items1Body := `{"data":{"items1":[{"id":"1"}]}}`
		items2Body := `{"data":{"items2":[{"id":"1"}]}}`
		searchBody := `{"data":{"search":[{"id":"1"}]}}`
		makeDS := func(t *testing.T, body string, schema string) []plan.DataSource {
			t.Helper()
			return []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t, "id",
					mustFactory(t,
						testNetHttpClient(t, roundTripperTestCase{
							expectedHost: "example.com", expectedPath: "/", expectedBody: "",
							sendResponseBody: body,
							sendStatusCode:   200,
						}),
					),
					&plan.DataSourceMetadata{
						RootNodes:  rootNodes,
						ChildNodes: childNodes,
						CostConfig: costConfig,
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						Fetch: &graphql_datasource.FetchConfiguration{
							URL:    "https://example.com/",
							Method: "GET",
						},
						SchemaConfiguration: mustSchemaConfig(t, nil, schema),
					}),
				),
			}
		}

		t.Run("single slicing arg supplied entirely by schema default is valid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ items1 { id } }`}
					},
					dataSources:           makeDS(t, items1Body, listSchema),
					fields:                fieldConfig,
					expectedResponse:      items1Body,
					expectedEstimatedCost: intPtr(15), // first default (5) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),
				},
				computeCosts(),
			)(t)
		})

		t.Run("flat slicing arg with omitted variables falls back to schema default", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query: `query ($limit: Int) { items1(first: $limit) { id } }`,
						}
					},
					dataSources:           makeDS(t, items1Body, listSchema),
					fields:                fieldConfig,
					expectedResponse:      items1Body,
					expectedEstimatedCost: intPtr(15), // first default (5) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),  // 1 * (Item(2)+Item.id(1))
				},
				computeCosts(),
			)(t)
		})

		t.Run("flat slicing arg with empty variable falls back to schema default", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query ($limit: Int) { items1(first: $limit) { id } }`,
							Variables: []byte(`{}`),
							// An absent variable is treated as omitted, schema default applies.
						}
					},
					dataSources:           makeDS(t, items1Body, listSchema),
					fields:                fieldConfig,
					expectedResponse:      items1Body,
					expectedEstimatedCost: intPtr(15), // first default (5) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),  // 1 * (Item(2)+Item.id(1))
				},
				computeCosts(),
			)(t)
		})

		t.Run("two slicing args, both supplied by schema defaults, are not valid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ items2 { id } }`}
					},
					dataSources: makeDS(t, items2Body, listSchema),
					fields:      fieldConfig,
				},
				"external: field 'Query.items2' requires exactly one slicing argument, but 2 were provided, locations: [], path: [items2]",
				computeCosts(),
			)(t)
		})

		t.Run("one explicit slicing arg and defaulted arg are invalid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ items2(first: 7) { id } }`}
					},
					dataSources: makeDS(t, items2Body, listSchema),
					fields:      fieldConfig,
				},
				"external: field 'Query.items2' requires exactly one slicing argument, but 2 were provided, locations: [], path: [items2]",
				computeCosts(),
			)(t)
		})

		t.Run("one explicit slicing arg and variable-nulled arg are valid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query ($n: Int) { items2(first: 7, last: $n) { id } }`,
							Variables: []byte(`{"n": null}`),
						}
					},
					dataSources:           makeDS(t, items2Body, listSchema),
					fields:                fieldConfig,
					expectedResponse:      items2Body,
					expectedEstimatedCost: intPtr(21), // first (7) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),
				},
				computeCosts(),
			)(t)
		})

		t.Run("one explicit slicing arg and nulled arg are valid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ items2(first: 7, last: null) { id } }`}
					},
					dataSources:           makeDS(t, items2Body, listSchema),
					fields:                fieldConfig,
					expectedResponse:      items2Body,
					expectedEstimatedCost: intPtr(21), // first default (7) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),
				},
				computeCosts(),
			)(t)
		})

		t.Run("dot-path slicing arg supplied by input field default is valid", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ search(input: {}) { id } }`}
					},
					dataSources:           makeDS(t, searchBody, listSchema),
					fields:                fieldConfig,
					expectedResponse:      searchBody,
					expectedEstimatedCost: intPtr(24), // Page.first default (8) * (Item(2)+Item.id(1))
					expectedActualCost:    intPtr(3),
				},
				computeCosts(),
			)(t)
		})

		t.Run("explicit null at dot-path leaf must not satisfy RequireOneSlicingArgument", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{Query: `{ search(input: { first: null }) { id } }`}
					},
					dataSources: makeDS(t, searchBody, listSchema),
					fields:      fieldConfig,
				},
				"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
				computeCosts(),
			)(t)
		})

		t.Run("explicit null at dot-path leaf variable must not satisfy RequireOneSlicingArgument", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query ($n: Int) { search(input: { first: $n }) { id } }`,
							Variables: []byte(`{"n": null}`),
						}
					},
					dataSources: makeDS(t, searchBody, listSchema),
					fields:      fieldConfig,
				},
				"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
				computeCosts(),
			)(t)
		})

		t.Run("explicit null at dot-path variable must not satisfy RequireOneSlicingArgument", func(t *testing.T) {
			schema, err := graphql.NewSchemaFromString(listSchema)
			require.NoError(t, err)
			runWithAndCompareError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query ($n: Page) { search(input: $n) { id } }`,
							Variables: []byte(`{"n": null}`),
						}
					},
					dataSources: makeDS(t, searchBody, listSchema),
					fields:      fieldConfig,
				},
				"external: field 'Query.search' requires exactly one slicing argument, but none was provided, locations: [], path: [search]",
				computeCosts(),
			)(t)
		})
	})

	t.Run("validate requireOneSlicingArgument on abstract types", func(t *testing.T) {
		t.Parallel()
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
				expectedEstimatedCost: intPtr(11), // Paginated(1) + 5 * (Item(2) + Item.id(0))
				expectedActualCost:    intPtr(3),  // Paginated(1) + 1 * (Item(2) + Item.id(0))
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

	t.Run("input object cost", func(t *testing.T) {
		t.Parallel()
		inputObjectSchema := `
			type Query {
				create(input: CreateInput!): User
				update(input: UpdateInput!): User
				nested(input: OuterInput!): Boolean
				recursive(input: RecursiveInput!): Boolean
				createList(input: CreateInput!): [User!] # @listSize(assumedSize: 10)
				listed(input: ListInput!): Boolean

				# inputOverride(input: CreateInput! @cost(weight: 7)): User @cost(weight: 1)
				inputOverride(input: CreateInput!): User
				discounted(input: DiscountedCreateInput!): Boolean
				negNested(input: NegNestedInput!): Boolean
				heavyDiscount(input: HeavyDiscountInput!): Boolean
				createMany(inputs: [CreateInput!]!): Boolean
			}
			type User {
				id: ID!
				name: String!
				email: String!
			}
			input CreateInput {
				name: String!          # @cost(weight: 5)
				email: String!         # @cost(weight: 3)
				age: Int               # @cost(weight: 2)
			}
			# nullable fields for null-value test
			input UpdateInput {
				name: String           # @cost(weight: 6)
				email: String          # @cost(weight: 4)
			}
			input OuterInput {
				label: String!         # @cost(weight: 2)
				inner: InnerInput!     # @cost(weight: 3)
			}
			input InnerInput {
				value: Int!            # @cost(weight: 4)
				note: String           # @cost(weight: 1)
			}
			input RecursiveInput {
				i: Int!                # @cost(weight: 2)
				rec: RecursiveInput    # @cost(weight: 3)
			}
			input ListInput {
				value: Int             # @cost(weight: 7)
				list: [OuterInput]
				rec: RecursiveInput
			}
			input DiscountedCreateInput {
				create: CreateInput!   # @cost(weight: -2) — wraps existing input type
				discount: Int          # @cost(weight: -3)
				priority: Int          # @cost(weight: 8)
			}
			input NegNestedInput {
				label: String!         # @cost(weight: 2)
				inner: NegInnerInput!  # @cost(weight: -1)
			}
			input NegInnerInput {
				value: Int!            # @cost(weight: 5)
				reduction: Int         # @cost(weight: -4)
			}
			input HeavyDiscountInput {
				base: Int!             # @cost(weight: 2)
				rebate: Int            # @cost(weight: -10)
			}
		`
		schema, err := graphql.NewSchemaFromString(inputObjectSchema)
		require.NoError(t, err)

		rootNodes := []plan.TypeField{
			{TypeName: "Query", FieldNames: []string{"create", "update", "nested", "recursive", "createList", "listed", "inputOverride", "discounted", "negNested", "heavyDiscount", "createMany"}},
			{TypeName: "User", FieldNames: []string{"id", "name", "email"}},
		}
		customConfig := mustConfiguration(t, graphql_datasource.ConfigurationInput{
			Fetch: &graphql_datasource.FetchConfiguration{
				URL:    "https://example.com/",
				Method: "GET",
			},
			SchemaConfiguration: mustSchemaConfig(t, nil, inputObjectSchema),
		})
		costConfig := &plan.DataSourceCostConfig{
			Weights: map[plan.FieldCoordinate]*plan.FieldCost{
				{TypeName: "CreateInput", FieldName: "name"}:   {HasWeight: true, Weight: 5},
				{TypeName: "CreateInput", FieldName: "email"}:  {HasWeight: true, Weight: 3},
				{TypeName: "CreateInput", FieldName: "age"}:    {HasWeight: true, Weight: 2},
				{TypeName: "UpdateInput", FieldName: "name"}:   {HasWeight: true, Weight: 6},
				{TypeName: "UpdateInput", FieldName: "email"}:  {HasWeight: true, Weight: 4},
				{TypeName: "OuterInput", FieldName: "label"}:   {HasWeight: true, Weight: 2},
				{TypeName: "OuterInput", FieldName: "inner"}:   {HasWeight: true, Weight: 3},
				{TypeName: "InnerInput", FieldName: "value"}:   {HasWeight: true, Weight: 4},
				{TypeName: "InnerInput", FieldName: "note"}:    {HasWeight: true, Weight: 1},
				{TypeName: "RecursiveInput", FieldName: "i"}:   {HasWeight: true, Weight: 2},
				{TypeName: "RecursiveInput", FieldName: "rec"}: {HasWeight: true, Weight: 3},
				{TypeName: "ListInput", FieldName: "value"}:    {HasWeight: true, Weight: 7},
				{TypeName: "Query", FieldName: "inputOverride"}: {
					HasWeight:       true,
					Weight:          1,
					ArgumentWeights: map[string]int{"input": 7},
				},
				{TypeName: "DiscountedCreateInput", FieldName: "create"}:   {HasWeight: true, Weight: -2},
				{TypeName: "DiscountedCreateInput", FieldName: "discount"}: {HasWeight: true, Weight: -3},
				{TypeName: "DiscountedCreateInput", FieldName: "priority"}: {HasWeight: true, Weight: 8},
				{TypeName: "NegNestedInput", FieldName: "label"}:           {HasWeight: true, Weight: 2},
				{TypeName: "NegNestedInput", FieldName: "inner"}:           {HasWeight: true, Weight: -1},
				{TypeName: "NegInnerInput", FieldName: "value"}:            {HasWeight: true, Weight: 5},
				{TypeName: "NegInnerInput", FieldName: "reduction"}:        {HasWeight: true, Weight: -4},
				{TypeName: "HeavyDiscountInput", FieldName: "base"}:        {HasWeight: true, Weight: 2},
				{TypeName: "HeavyDiscountInput", FieldName: "rebate"}:      {HasWeight: true, Weight: -10},
			},
			ListSizes: map[plan.FieldCoordinate]*plan.FieldListSize{
				{TypeName: "Query", FieldName: "createList"}: {AssumedSize: 10},
			},
		}
		fieldConfig := []plan.FieldConfiguration{
			{
				TypeName: "Query", FieldName: "create",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "update",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "nested",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "recursive",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "createList",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "listed",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "inputOverride",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "discounted",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "negNested",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "heavyDiscount",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "input", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
			{
				TypeName: "Query", FieldName: "createMany",
				Arguments: []plan.ArgumentConfiguration{
					{Name: "inputs", SourceType: plan.FieldArgumentSource, RenderConfig: plan.RenderArgumentAsGraphQLValue},
				},
			},
		}

		t.Run("basic input object with weighted fields inline", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ create(input: {name: "Alice", email: "a@b.com", age: 30}) { id name } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"create":{"id":"1","name":"Alice"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"create":{"id":"1","name":"Alice"}}}`,
				expectedEstimatedCost: intPtr(11), // argsCost(name:5 + email:3 + age:2 = 10) + round((0 + 1) * 1) = 11
				expectedActualCost:    intPtr(11),
			},
			computeCosts(),
		))

		t.Run("input object via variable with partial fields", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query:     `query($input: CreateInput!) { create(input: $input) { id name } }`,
						Variables: []byte(`{"input": {"name": "Alice", "email": "a@b.com"}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"create":{"id":"1","name":"Alice"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"create":{"id":"1","name":"Alice"}}}`,
				expectedEstimatedCost: intPtr(9), // argsCost(name:5 + email:3 = 8, age omitted) + round((0 + 1) * 1) = 9
				expectedActualCost:    intPtr(9),
			},
			computeCosts(),
		))

		t.Run("nested input objects", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ nested(input: {label: "hello", inner: {value: 42, note: "n"}}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"nested":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"nested":true}}`,
				expectedEstimatedCost: intPtr(10), // argsCost(label:2 + inner:3 + value:4 + note:1 = 10) + round((0 + 0) * 1) = 10
				expectedActualCost:    intPtr(10),
			},
			computeCosts(),
		))

		t.Run("recursive input objects", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ recursive(input: {i: 1, rec: {i: 2, rec: {i: 3}}}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"recursive":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:           fieldConfig,
				expectedResponse: `{"data":{"recursive":true}}`,

				// countedInputCoords = {A.i: 3, A.rec: 2} implies
				// argsCost(3*2 + 2*3 = 12) = 12
				expectedEstimatedCost: intPtr(12),
				expectedActualCost:    intPtr(12),
			},
			computeCosts(),
		))

		t.Run("input object on list field argsCost not multiplied", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ createList(input: {name: "Eve", email: "e@f.com"}) { id name } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"createList":[{"id":"1","name":"Eve"},{"id":"2","name":"Eve2"}]}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"createList":[{"id":"1","name":"Eve"},{"id":"2","name":"Eve2"}]}}`,
				expectedEstimatedCost: intPtr(18), // argsCost(name:5 + email:3 = 8) + round((0 + 1) * 10) = 18
				expectedActualCost:    intPtr(10), // argsCost(8) + round((0 + 1) * 2) = 10
			},
			computeCosts(),
		))

		t.Run("explicit argument weight does not override input object cost", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ inputOverride(input: {name: "Alice", email: "a@b.com", age: 30}) { id } }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"inputOverride":{"id":"1"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"inputOverride":{"id":"1"}}}`,
				expectedEstimatedCost: intPtr(18), // argsCost(input:7 + name:5 + email:3 + age:2 = 17) + round((0 + 1) * 1) = 18
				expectedActualCost:    intPtr(18),
			},
			computeCosts(),
		))

		t.Run("null field values not counted", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query:     `query($input: UpdateInput!) { update(input: $input) { id } }`,
						Variables: []byte(`{"input": {"name": "Bob", "email": null}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"update":{"id":"1"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"update":{"id":"1"}}}`,
				expectedEstimatedCost: intPtr(7), // argsCost(name:6) + round((0 + 1) * 1) = 7
				expectedActualCost:    intPtr(7),
			},
			computeCosts(),
		))

		t.Run("all fields null via variable", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query:     `query($input: UpdateInput!) { update(input: $input) { id } }`,
						Variables: []byte(`{"input": {"name": null, "email": null}}`),
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"update":{"id":"1"}}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"update":{"id":"1"}}}`,
				expectedEstimatedCost: intPtr(1), // argsCost(0) + round((0 + 1) * 1) = 1
				expectedActualCost:    intPtr(1),
			},
			computeCosts(),
		))

		t.Run("listed field with list of input objects", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ listed(input: {
									value: 5, 
									list: [
										{
											label: "a", 
											inner: {value: 1}
										}, 
										{
											label: "b", 
											inner: {value: 2}
										}
									]
								})}`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"listed":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"listed":true}}`,
				expectedEstimatedCost: intPtr(25), // 7 + 5*2 + 4*2
				expectedActualCost:    intPtr(25),
			},
			computeCosts(),
		))
		t.Run("negative input field weights reduce cost", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ discounted(input: {create: {name: "A", email: "b"}, discount: 5, priority: 1}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"discounted":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"discounted":true}}`,
				expectedEstimatedCost: intPtr(11), // create:(name:5 + email:3) + create:-2 + discount:-3 + priority:8
				expectedActualCost:    intPtr(11),
			},
			computeCosts(),
		))

		t.Run("omitting negative input field gives higher cost", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ discounted(input: {create: {name: "A", email: "b"}, priority: 1}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"discounted":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"discounted":true}}`,
				expectedEstimatedCost: intPtr(14), // create→(name:5 + email:3) + create:-2 + priority:8 = 14 (no discount:-3)
				expectedActualCost:    intPtr(14),
			},
			computeCosts(),
		))

		t.Run("negative weights in nested input object", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ negNested(input: {label: "x", inner: {value: 42, reduction: 1}}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"negNested":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"negNested":true}}`,
				expectedEstimatedCost: intPtr(2), // label:2 + inner→(value:5 + reduction:-4) + inner:-1 = 2
				expectedActualCost:    intPtr(2),
			},
			computeCosts(),
		))

		t.Run("omitting negative nested field gives higher cost", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ negNested(input: {label: "x", inner: {value: 42}}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"negNested":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"negNested":true}}`,
				expectedEstimatedCost: intPtr(6), // label:2 + inner→(value:5) + inner:-1 = 6 (no reduction:-4)
				expectedActualCost:    intPtr(6),
			},
			computeCosts(),
		))

		t.Run("negative cost clamped to zero", runWithoutError(
			ExecutionEngineTestCase{
				schema: schema,
				operation: func(t *testing.T) graphql.Request {
					return graphql.Request{
						Query: `{ heavyDiscount(input: {base: 1, rebate: 1}) }`,
					}
				},
				dataSources: []plan.DataSource{
					mustGraphqlDataSourceConfiguration(t, "id",
						mustFactory(t,
							testNetHttpClient(t, roundTripperTestCase{
								expectedHost: "example.com", expectedPath: "/", expectedBody: "",
								sendResponseBody: `{"data":{"heavyDiscount":true}}`,
								sendStatusCode:   200,
							}),
						),
						&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
						customConfig,
					),
				},
				fields:                fieldConfig,
				expectedResponse:      `{"data":{"heavyDiscount":true}}`,
				expectedEstimatedCost: intPtr(0), // base:2 + rebate:-10 = -8 → floored to 0
				expectedActualCost:    intPtr(0),
			},
			computeCosts(),
		))

		t.Run("list-typed input object argument", func(t *testing.T) {
			t.Run("should count per-item field weights", runWithoutError(
				ExecutionEngineTestCase{
					schema: schema,
					operation: func(t *testing.T) graphql.Request {
						return graphql.Request{
							Query:     `query($inputs: [CreateInput!]!) { createMany(inputs: $inputs) }`,
							Variables: []byte(`{"inputs": [{"name": "A", "email": "a@b.com"}, {"name": "B", "email": "b@c.com", "age": 30}]}`),
						}
					},
					dataSources: []plan.DataSource{
						mustGraphqlDataSourceConfiguration(t, "id",
							mustFactory(t,
								testNetHttpClient(t, roundTripperTestCase{
									expectedHost: "example.com", expectedPath: "/", expectedBody: "",
									sendResponseBody: `{"data":{"createMany":true}}`,
									sendStatusCode:   200,
								}),
							),
							&plan.DataSourceMetadata{RootNodes: rootNodes, CostConfig: costConfig},
							customConfig,
						),
					},
					fields:                fieldConfig,
					expectedResponse:      `{"data":{"createMany":true}}`,
					expectedEstimatedCost: intPtr(18), // item1(name:5 + email:3) + item2(name:5 + email:3 + age:2) = 18
					expectedActualCost:    intPtr(18),
				},
				computeCosts(),
			))
		})
	})
}
