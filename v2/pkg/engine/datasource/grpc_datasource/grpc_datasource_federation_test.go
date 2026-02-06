package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

func Test_DataSource_Load_WithEntity_Calls(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]interface{} `json:"data"`
		Errors []graphqlError         `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]interface{})
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			name:  "Query nullable fields type with all fields",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Product { id name } ...on Storage { id name } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Product","id":"1"},
				{"__typename":"Storage","id":"3"},
				{"__typename":"Product","id":"2"},
				{"__typename":"Storage","id":"4"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")

				// Check required fields are present
				require.Contains(t, entities[0], "id")
				require.Contains(t, entities[0], "name")
				require.Contains(t, entities[1], "id")
				require.Contains(t, entities[1], "name")

				require.Len(t, entities, 4, "Should return 4 entities")

				product, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "product should be an object")
				require.Equal(t, "1", product["id"])
				require.Equal(t, "Product 1", product["name"])

				storage, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage should be an object")
				require.Equal(t, "3", storage["id"])
				require.Equal(t, "Storage 3", storage["name"])

				product2, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "product2 should be an object")
				require.Equal(t, "2", product2["id"])
				require.Equal(t, "Product 2", product2["name"])

				storage2, ok := entities[3].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "4", storage2["id"])
				require.Equal(t, "Storage 4", storage2["name"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query warehouse and expect an error",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Warehouse { id name } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Warehouse","id":"1"},
				{"__typename":"Warehouse","id":"2"},
				{"__typename":"Warehouse","id":"3"},
				{"__typename":"Warehouse","id":"4"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Warehouse",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				require.Empty(t, data)
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.NotEmpty(t, errorData)
				require.Equal(t, "entity type Warehouse received 3 entities in the subgraph response, but 4 are expected", errorData[0].Message)
			},
		},
		{
			name:  "Query Product with field resolvers",
			query: `query($representations: [_Any!]!, $input: ShippingEstimateInput!) { _entities(representations: $representations) { ...on Product { id name price shippingEstimate(input: $input) } } }`,
			vars: `
			{
			  "variables":
			  {
			    "representations":[
				  {"__typename":"Product","id":"1"},
				  {"__typename":"Product","id":"2"},
				  {"__typename":"Product","id":"3"}
				],
				"input":{
				  "destination":"INTERNATIONAL",
				  "weight":10.0,
				  "expedited":true
				}
			}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")
				for index, entity := range entities {
					entity, ok := entity.(map[string]interface{})
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])
					require.InDelta(t, float64(99.99), entity["price"], 0.01)
					require.InDelta(t, float64(77.49), entity["shippingEstimate"], 0.01)
				}

			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			if err != nil {
				t.Fatalf("failed to compile proto: %v", err)
			}

			// Create the datasource
			ds, err := NewDataSource(conn, DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

func Test_DataSource_Load_WithEntity_Calls_WithCompositeTypes(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]interface{} `json:"data"`
		Errors []graphqlError         `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]interface{})
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			name:  "Query Product with field resolver returning interface type",
			query: `query($representations: [_Any!]!, $includeDetails: Boolean!) { _entities(representations: $representations) { ...on Product { __typename id name mascotRecommendation(includeDetails: $includeDetails) { ... on Cat { __typename name meowVolume } ... on Dog { __typename name barkVolume } } } } }`,
			vars: `{
				"variables": {
					"representations": [
						{"__typename":"Product","id":"1"},
						{"__typename":"Product","id":"2"},
						{"__typename":"Product","id":"3"}
					],
					"includeDetails": true
				}
			}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]interface{})
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					mascot, ok := entity["mascotRecommendation"].(map[string]interface{})
					require.True(t, ok, "mascotRecommendation should be an object")

					// Alternates between Cat and Dog based on index
					if index%2 == 0 {
						// Should be Cat
						typename, ok := mascot["__typename"].(string)
						require.True(t, ok, "__typename should be present")
						require.Equal(t, "Cat", typename)

						require.Contains(t, mascot, "name")
						require.Contains(t, mascot["name"], "MascotCat")

						// Validate meowVolume field
						require.Contains(t, mascot, "meowVolume")
						meowVolume, ok := mascot["meowVolume"].(float64)
						require.True(t, ok, "meowVolume should be a number")
						require.Greater(t, meowVolume, float64(0), "meowVolume should be greater than 0")
					} else {
						// Should be Dog
						typename, ok := mascot["__typename"].(string)
						require.True(t, ok, "__typename should be present")
						require.Equal(t, "Dog", typename)

						require.Contains(t, mascot, "name")
						require.Contains(t, mascot["name"], "MascotDog")

						// Validate barkVolume field
						require.Contains(t, mascot, "barkVolume")
						barkVolume, ok := mascot["barkVolume"].(float64)
						require.True(t, ok, "barkVolume should be a number")
						require.Greater(t, barkVolume, float64(0), "barkVolume should be greater than 0")
					}
				}
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Product with field resolver returning union type",
			query: `query($representations: [_Any!]!, $checkAvailability: Boolean!) { _entities(representations: $representations) { ...on Product { __typename id name stockStatus(checkAvailability: $checkAvailability) { ... on ActionSuccess { __typename message timestamp } ... on ActionError { __typename message code } } } } }`,
			vars: `{
				"variables": {
					"representations": [
						{"__typename":"Product","id":"1"},
						{"__typename":"Product","id":"2"},
						{"__typename":"Product","id":"3"}
					],
					"checkAvailability": false
				}
			}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]interface{})
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					stockStatus, ok := entity["stockStatus"].(map[string]interface{})
					require.True(t, ok, "stockStatus should be an object")

					// With checkAvailability: false, all should be success
					typename, ok := stockStatus["__typename"].(string)
					require.True(t, ok, "__typename should be present")
					require.Equal(t, "ActionSuccess", typename)

					require.Contains(t, stockStatus, "message")
					require.Contains(t, stockStatus, "timestamp")

					message, ok := stockStatus["message"].(string)
					require.True(t, ok, "message should be a string")
					require.Contains(t, message, "in stock and available")

					timestamp, ok := stockStatus["timestamp"].(string)
					require.True(t, ok, "timestamp should be a string")
					require.NotEmpty(t, timestamp)
				}
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Product with field resolver returning nested composite types",
			query: `query($representations: [_Any!]!, $includeExtended: Boolean!) { _entities(representations: $representations) { ...on Product { __typename id name price productDetails(includeExtended: $includeExtended) { id description recommendedPet { __typename ... on Cat { name meowVolume } ... on Dog { name barkVolume } } reviewSummary { __typename ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } } }`,
			vars: `{
				"variables": {
					"representations": [
						{"__typename":"Product","id":"1"},
						{"__typename":"Product","id":"2"}
					],
					"includeExtended": false
				}
			}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 2, "Should return 2 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]interface{})
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					details, ok := entity["productDetails"].(map[string]interface{})
					require.True(t, ok, "productDetails should be an object")

					require.Contains(t, details, "id")
					require.Contains(t, details, "description")
					require.Contains(t, details["description"], "Standard details")

					// Check recommendedPet (interface)
					pet, ok := details["recommendedPet"].(map[string]interface{})
					require.True(t, ok, "recommendedPet should be an object")

					// Alternates between Cat and Dog
					if index%2 == 0 {
						// Should be Cat
						petTypename, ok := pet["__typename"].(string)
						require.True(t, ok, "pet __typename should be present")
						require.Equal(t, "Cat", petTypename)

						require.Contains(t, pet, "name")
						require.Contains(t, pet["name"], "RecommendedCat")

						// Validate meowVolume field
						require.Contains(t, pet, "meowVolume")
						meowVolume, ok := pet["meowVolume"].(float64)
						require.True(t, ok, "meowVolume should be a number")
						require.Greater(t, meowVolume, float64(0), "meowVolume should be greater than 0")
					} else {
						// Should be Dog
						petTypename, ok := pet["__typename"].(string)
						require.True(t, ok, "pet __typename should be present")
						require.Equal(t, "Dog", petTypename)

						require.Contains(t, pet, "name")
						require.Contains(t, pet["name"], "RecommendedDog")

						// Validate barkVolume field
						require.Contains(t, pet, "barkVolume")
						barkVolume, ok := pet["barkVolume"].(float64)
						require.True(t, ok, "barkVolume should be a number")
						require.Greater(t, barkVolume, float64(0), "barkVolume should be greater than 0")
					}

					// Check reviewSummary (union)
					reviewSummary, ok := details["reviewSummary"].(map[string]interface{})
					require.True(t, ok, "reviewSummary should be an object")

					// With includeExtended: false and low prices, should be success
					reviewTypename, ok := reviewSummary["__typename"].(string)
					require.True(t, ok, "reviewSummary __typename should be present")
					require.Equal(t, "ActionSuccess", reviewTypename)

					require.Contains(t, reviewSummary, "message")
					require.Contains(t, reviewSummary, "timestamp")

					message, ok := reviewSummary["message"].(string)
					require.True(t, ok, "message should be a string")
					require.Contains(t, message, "positive reviews")

					timestamp, ok := reviewSummary["timestamp"].(string)
					require.True(t, ok, "timestamp should be a string")
					require.NotEmpty(t, timestamp)
				}
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			if err != nil {
				t.Fatalf("failed to compile proto: %v", err)
			}

			// Create the datasource
			ds, err := NewDataSource(conn, DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

func Test_DataSource_Load_WithEntity_Calls_And_Requires(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]interface{} `json:"data"`
		Errors []graphqlError         `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]interface{})
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			/*
				type Storage @key(fields: "id") {
				id: ID!
				name: String!
				location: String!
				itemCount: Int! @external
				restockData: RestockData! @external
				stockHealthScore: Float! @requires(fields: "itemCount restockData { lastRestockDate }")
				}
			*/
			name:  "Query Storage type with required field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"lastRestockDate":"2021-01-01"}},
				{"__typename":"Storage","id":"2","itemCount":200,"restockData":{"lastRestockDate":"2021-01-02"}},
				{"__typename":"Storage","id":"3","itemCount":300,"restockData":{"lastRestockDate":"2021-01-03"}},
				{"__typename":"Storage","id":"4","itemCount":400,"restockData":{"lastRestockDate":"2021-01-04"}},
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "stockHealthScore",
					SelectionSet: "itemCount restockData { lastRestockDate }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// Storage 1: itemCount=100, restockData provided -> score = 100*0.1 + 10 = 20.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, 20.0, storage1["stockHealthScore"])

				// Storage 2: itemCount=200, restockData provided -> score = 200*0.1 + 10 = 30.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, 30.0, storage2["stockHealthScore"])

				// Storage 3: itemCount=300, restockData provided -> score = 300*0.1 + 10 = 40.0
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "Storage 3", storage3["name"])
				require.Equal(t, 40.0, storage3["stockHealthScore"])

				// Storage 4: itemCount=400, restockData provided -> score = 400*0.1 + 10 = 50.0
				storage4, ok := entities[3].(map[string]interface{})
				require.True(t, ok, "storage4 should be an object")
				require.Equal(t, "4", storage4["id"])
				require.Equal(t, "Storage 4", storage4["name"])
				require.Equal(t, 50.0, storage4["stockHealthScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with empty restockData (no +10 bonus)",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"lastRestockDate":""}},
				{"__typename":"Storage","id":"2","itemCount":500,"restockData":{"lastRestockDate":""}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "stockHealthScore",
					SelectionSet: "itemCount restockData { lastRestockDate }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: itemCount=100, no restockData -> score = 100*0.1 = 10.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, 10.0, storage1["stockHealthScore"])

				// Storage 2: itemCount=500, no restockData -> score = 500*0.1 = 50.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, 50.0, storage2["stockHealthScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query single Storage entity with required field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"42","itemCount":1000,"restockData":{"lastRestockDate":"2024-06-15"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "stockHealthScore",
					SelectionSet: "itemCount restockData { lastRestockDate }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 1, "Should return 1 entity")

				// Storage 42: itemCount=1000, restockData provided -> score = 1000*0.1 + 10 = 110.0
				storage, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage should be an object")
				require.Equal(t, "42", storage["id"])
				require.Equal(t, "Storage 42", storage["name"])
				require.Equal(t, 110.0, storage["stockHealthScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage without stockHealthScore field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1"},
				{"__typename":"Storage","id":"2"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Just id and name, no stockHealthScore
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.NotContains(t, storage1, "stockHealthScore")

				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.NotContains(t, storage2, "stockHealthScore")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with tagSummary requiring tags list",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name tagSummary } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["electronics","gadgets","sale"]},
				{"__typename":"Storage","id":"2","tags":["books","fiction"]},
				{"__typename":"Storage","id":"3","tags":[]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: tags = ["electronics", "gadgets", "sale"] -> "electronics, gadgets, sale"
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "electronics, gadgets, sale", storage1["tagSummary"])

				// Storage 2: tags = ["books", "fiction"] -> "books, fiction"
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "books, fiction", storage2["tagSummary"])

				// Storage 3: tags = [] -> ""
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "Storage 3", storage3["name"])
				require.Equal(t, "", storage3["tagSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalTagSummary requiring nullable tags list",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalTagSummary } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","optionalTags":["premium","featured"]},
				{"__typename":"Storage","id":"2","optionalTags":[]},
				{"__typename":"Storage","id":"3","optionalTags":null}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: optionalTags = ["premium", "featured"] -> "premium, featured"
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium, featured", storage1["optionalTagSummary"])

				// Storage 2: optionalTags = [] -> null (empty list returns nil)
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalTagSummary"])

				// Storage 3: optionalTags = null -> null
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalTagSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with metadataScore requiring nested object",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id metadataScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":100,"zone":"A"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":200,"zone":"B"}},
				{"__typename":"Storage","id":"3","metadata":{"capacity":300,"zone":"C"}},
				{"__typename":"Storage","id":"4","metadata":{"capacity":400,"zone":"D"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// Storage 1: capacity=100, zone="A" (weight=1.0) -> 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, 100.0, storage1["metadataScore"])

				// Storage 2: capacity=200, zone="B" (weight=0.8) -> 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, 160.0, storage2["metadataScore"])

				// Storage 3: capacity=300, zone="C" (weight=0.6) -> 300*0.6 = 180.0
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, 180.0, storage3["metadataScore"])

				// Storage 4: capacity=400, zone="D" (weight=0.5) -> 400*0.5 = 200.0
				storage4, ok := entities[3].(map[string]interface{})
				require.True(t, ok, "storage4 should be an object")
				require.Equal(t, "4", storage4["id"])
				require.Equal(t, 200.0, storage4["metadataScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with processedMetadata returning complex type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id processedMetadata { capacity zone priority } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":50,"zone":"a","priority":5}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":100,"zone":"b","priority":10}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadata",
					SelectionSet: "metadata { capacity zone priority }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=50*2=100, zone="A" (uppercase), priority=5+10=15
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["processedMetadata"].(map[string]interface{})
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(100), metadata1["capacity"])
				require.Equal(t, "A", metadata1["zone"])
				require.Equal(t, float64(15), metadata1["priority"])

				// Storage 2: capacity=100*2=200, zone="B" (uppercase), priority=10+10=20
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				metadata2, ok := storage2["processedMetadata"].(map[string]interface{})
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(200), metadata2["capacity"])
				require.Equal(t, "B", metadata2["zone"])
				require.Equal(t, float64(20), metadata2["priority"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalProcessedMetadata returning nullable complex type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalProcessedMetadata { capacity zone } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":100,"zone":"X"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":200,"zone":"Y"}},
				{"__typename":"Storage","id":"3","metadata":{"capacity":300,"zone":"Z"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedMetadata",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even): returns processed metadata
				// capacity=100*3=300, zone="x" (lowercase), priority=1
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["optionalProcessedMetadata"].(map[string]interface{})
				require.True(t, ok, "optionalProcessedMetadata should be an object for index 0")
				require.Equal(t, float64(300), metadata1["capacity"])
				require.Equal(t, "x", metadata1["zone"])

				// Storage 2 (index 1, odd): returns null
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalProcessedMetadata"])

				// Storage 3 (index 2, even): returns processed metadata
				// capacity=300*3=900, zone="z" (lowercase), priority=1
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				metadata3, ok := storage3["optionalProcessedMetadata"].(map[string]interface{})
				require.True(t, ok, "optionalProcessedMetadata should be an object for index 2")
				require.Equal(t, float64(900), metadata3["capacity"])
				require.Equal(t, "z", metadata3["zone"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with processedTags returning list",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id processedTags } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["foo","bar"]},
				{"__typename":"Storage","id":"2","tags":["hello"]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedTags",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tags = ["foo", "bar"] -> ["PROCESSED_FOO", "PROCESSED_BAR"]
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["processedTags"].([]interface{})
				require.True(t, ok, "processedTags should be an array")
				require.Len(t, tags1, 2)
				require.Equal(t, "PROCESSED_FOO", tags1[0])
				require.Equal(t, "PROCESSED_BAR", tags1[1])

				// Storage 2: tags = ["hello"] -> ["PROCESSED_HELLO"]
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				tags2, ok := storage2["processedTags"].([]interface{})
				require.True(t, ok, "processedTags should be an array")
				require.Len(t, tags2, 1)
				require.Equal(t, "PROCESSED_HELLO", tags2[0])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalProcessedTags returning nullable list",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalProcessedTags } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","optionalTags":["Alpha","Beta"]},
				{"__typename":"Storage","id":"2","optionalTags":["Gamma"]},
				{"__typename":"Storage","id":"3","optionalTags":[]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedTags",
					SelectionSet: "optionalTags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even with data): returns ["opt_alpha", "opt_beta"]
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["optionalProcessedTags"].([]interface{})
				require.True(t, ok, "optionalProcessedTags should be an array for index 0")
				require.Len(t, tags1, 2)
				require.Equal(t, "OPT_alpha", tags1[0])
				require.Equal(t, "OPT_beta", tags1[1])

				// Storage 2 (index 1, odd): returns null
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalProcessedTags"])

				// Storage 3 (index 2, even but empty): returns null (empty list returns nil)
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalProcessedTags"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with processedMetadataHistory returning list of complex types",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id processedMetadataHistory { capacity zone priority } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadataHistory":[{"capacity":10,"zone":"A"},{"capacity":20,"zone":"B"}]},
				{"__typename":"Storage","id":"2","metadataHistory":[{"capacity":100,"zone":"X"}]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadataHistory",
					SelectionSet: "metadataHistory { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: history with 2 items
				// Item 0: capacity=10*1=10, zone="HIST_A", priority=1
				// Item 1: capacity=20*2=40, zone="HIST_B", priority=2
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				history1, ok := storage1["processedMetadataHistory"].([]interface{})
				require.True(t, ok, "processedMetadataHistory should be an array")
				require.Len(t, history1, 2)

				item0, ok := history1[0].(map[string]interface{})
				require.True(t, ok, "history item should be an object")
				require.Equal(t, float64(10), item0["capacity"])
				require.Equal(t, "HIST_A", item0["zone"])
				require.Equal(t, float64(1), item0["priority"])

				item1, ok := history1[1].(map[string]interface{})
				require.True(t, ok, "history item should be an object")
				require.Equal(t, float64(40), item1["capacity"])
				require.Equal(t, "HIST_B", item1["zone"])
				require.Equal(t, float64(2), item1["priority"])

				// Storage 2: history with 1 item
				// Item 0: capacity=100*1=100, zone="HIST_X", priority=1
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				history2, ok := storage2["processedMetadataHistory"].([]interface{})
				require.True(t, ok, "processedMetadataHistory should be an array")
				require.Len(t, history2, 1)

				item2, ok := history2[0].(map[string]interface{})
				require.True(t, ok, "history item should be an object")
				require.Equal(t, float64(100), item2["capacity"])
				require.Equal(t, "HIST_X", item2["zone"])
				require.Equal(t, float64(1), item2["priority"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with multiple requires fields in single query",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name tagSummary metadataScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["tech","sale"],"metadata":{"capacity":100,"zone":"A"}},
				{"__typename":"Storage","id":"2","tags":["books"],"metadata":{"capacity":200,"zone":"B"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tagSummary = "tech, sale", metadataScore = 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "tech, sale", storage1["tagSummary"])
				require.Equal(t, 100.0, storage1["metadataScore"])

				// Storage 2: tagSummary = "books", metadataScore = 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "books", storage2["tagSummary"])
				require.Equal(t, 160.0, storage2["metadataScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			if err != nil {
				t.Fatalf("failed to compile proto: %v", err)
			}

			// Create the datasource
			ds, err := NewDataSource(conn, DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

func Test_DataSource_Load_WithEntity_Calls_And_Requires_And_FieldResolvers(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]interface{} `json:"data"`
		Errors []graphqlError         `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]interface{})
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			name:  "Query Storage with tagSummary (requires) + storageStatus (field resolver)",
			query: `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message } ... on ActionError { message code } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["electronics","gadgets","sale"]},
				{"__typename":"Storage","id":"2","tags":["books","fiction"]},
				{"__typename":"Storage","id":"3","tags":[]}
			],"checkHealth":false}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: tags = ["electronics", "gadgets", "sale"] -> "electronics, gadgets, sale"
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "electronics, gadgets, sale", storage1["tagSummary"])
				// Check storageStatus field resolver result
				status1, ok := storage1["storageStatus"].(map[string]interface{})
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status1, "message")
				require.Contains(t, status1["message"], "is healthy")

				// Storage 2: tags = ["books", "fiction"] -> "books, fiction"
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "books, fiction", storage2["tagSummary"])
				status2, ok := storage2["storageStatus"].(map[string]interface{})
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status2, "message")

				// Storage 3: tags = [] -> ""
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "", storage3["tagSummary"])
				status3, ok := storage3["storageStatus"].(map[string]interface{})
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status3, "message")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with metadataScore (requires) + linkedStorages (field resolver)",
			query: `query($representations: [_Any!]!, $depth: Int!) { _entities(representations: $representations) { ...on Storage { __typename id metadataScore linkedStorages(depth: $depth) { id name } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":100,"zone":"A"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":200,"zone":"B"}}
			],"depth":2}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=100, zone="A" (weight=1.0) -> 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, 100.0, storage1["metadataScore"])
				// Check linkedStorages field resolver result
				linked1, ok := storage1["linkedStorages"].([]interface{})
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked1, 2, "Should return 2 linked storages (depth=2)")
				for i, linked := range linked1 {
					linkedStorage, ok := linked.(map[string]interface{})
					require.True(t, ok, "linked storage should be an object")
					require.Contains(t, linkedStorage["id"], fmt.Sprintf("linked-storage-1-%d", i))
					require.Contains(t, linkedStorage, "name")
				}

				// Storage 2: capacity=200, zone="B" (weight=0.8) -> 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, 160.0, storage2["metadataScore"])
				linked2, ok := storage2["linkedStorages"].([]interface{})
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked2, 2, "Should return 2 linked storages (depth=2)")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalTagSummary (nullable requires) + nearbyStorages (nullable field resolver)",
			query: `query($representations: [_Any!]!, $radius: Int) { _entities(representations: $representations) { ...on Storage { __typename id optionalTagSummary nearbyStorages(radius: $radius) { id name } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","optionalTags":["premium","featured"]},
				{"__typename":"Storage","id":"2","optionalTags":[]},
				{"__typename":"Storage","id":"3","optionalTags":null}
			],"radius":3}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: optionalTags = ["premium", "featured"] -> "premium, featured"
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium, featured", storage1["optionalTagSummary"])
				// Check nearbyStorages field resolver result with radius=3
				nearby1, ok := storage1["nearbyStorages"].([]interface{})
				require.True(t, ok, "nearbyStorages should be an array")
				require.Len(t, nearby1, 3, "Should return 3 nearby storages (radius=3)")

				// Storage 2: optionalTags = [] -> null (empty list returns nil)
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalTagSummary"])
				nearby2, ok := storage2["nearbyStorages"].([]interface{})
				require.True(t, ok, "nearbyStorages should be an array")
				require.Len(t, nearby2, 3, "Should return 3 nearby storages (radius=3)")

				// Storage 3: optionalTags = null -> null
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalTagSummary"])
				nearby3, ok := storage3["nearbyStorages"].([]interface{})
				require.True(t, ok, "nearbyStorages should be an array")
				require.Len(t, nearby3, 3, "Should return 3 nearby storages (radius=3)")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalTagSummary (nullable requires) + nearbyStorages (null radius - tests null behavior)",
			query: `query($representations: [_Any!]!, $radius: Int) { _entities(representations: $representations) { ...on Storage { __typename id optionalTagSummary nearbyStorages(radius: $radius) { id name } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","optionalTags":["premium"]},
				{"__typename":"Storage","id":"2","optionalTags":["featured"]},
				{"__typename":"Storage","id":"3","optionalTags":["sale"]},
				{"__typename":"Storage","id":"4","optionalTags":["discount"]}
			],"radius":null}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalTagSummary",
					SelectionSet: "optionalTags",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// When radius is null, the mock service behavior is:
				// - Even indices (0, 2): return empty list
				// - Odd indices (1, 3): return null

				// Storage 1 (index 0, even): nearbyStorages should be empty list
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium", storage1["optionalTagSummary"])
				nearby1, ok := storage1["nearbyStorages"].([]interface{})
				require.True(t, ok, "nearbyStorages should be an empty array for even index")
				require.Len(t, nearby1, 0, "Should return empty list for index 0 when radius is null")

				// Storage 2 (index 1, odd): nearbyStorages should be null
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "featured", storage2["optionalTagSummary"])
				require.Nil(t, storage2["nearbyStorages"], "nearbyStorages should be null for odd index")

				// Storage 3 (index 2, even): nearbyStorages should be empty list
				storage3, ok := entities[2].(map[string]interface{})
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "sale", storage3["optionalTagSummary"])
				nearby3, ok := storage3["nearbyStorages"].([]interface{})
				require.True(t, ok, "nearbyStorages should be an empty array for even index")
				require.Len(t, nearby3, 0, "Should return empty list for index 2 when radius is null")

				// Storage 4 (index 3, odd): nearbyStorages should be null
				storage4, ok := entities[3].(map[string]interface{})
				require.True(t, ok, "storage4 should be an object")
				require.Equal(t, "4", storage4["id"])
				require.Equal(t, "discount", storage4["optionalTagSummary"])
				require.Nil(t, storage4["nearbyStorages"], "nearbyStorages should be null for odd index")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with multiple requires (tagSummary + metadataScore) + storageStatus (field resolver)",
			query: `query($representations: [_Any!]!, $checkHealth: Boolean!) { _entities(representations: $representations) { ...on Storage { __typename id tagSummary metadataScore storageStatus(checkHealth: $checkHealth) { ... on ActionSuccess { message timestamp } ... on ActionError { message code } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["tech","sale"],"metadata":{"capacity":100,"zone":"A"}},
				{"__typename":"Storage","id":"2","tags":["books"],"metadata":{"capacity":200,"zone":"B"}}
			],"checkHealth":false}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagSummary",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "metadataScore",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tagSummary = "tech, sale", metadataScore = 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "tech, sale", storage1["tagSummary"])
				require.Equal(t, 100.0, storage1["metadataScore"])
				// Check storageStatus field resolver result
				status1, ok := storage1["storageStatus"].(map[string]interface{})
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status1, "message")
				require.Contains(t, status1["message"], "is healthy")
				require.Contains(t, status1, "timestamp")

				// Storage 2: tagSummary = "books", metadataScore = 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "books", storage2["tagSummary"])
				require.Equal(t, 160.0, storage2["metadataScore"])
				status2, ok := storage2["storageStatus"].(map[string]interface{})
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status2, "message")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with processedMetadata (complex return requires) + linkedStorages (field resolver)",
			query: `query($representations: [_Any!]!, $depth: Int!) { _entities(representations: $representations) { ...on Storage { __typename id processedMetadata { capacity zone priority } linkedStorages(depth: $depth) { id name } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":50,"zone":"a","priority":5}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":100,"zone":"b","priority":10}}
			],"depth":1}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "processedMetadata",
					SelectionSet: "metadata { capacity zone priority }",
				},
			},
			validate: func(t *testing.T, data map[string]interface{}) {
				entities, ok := data["_entities"].([]interface{})
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=50*2=100, zone="A" (uppercase), priority=5+10=15
				storage1, ok := entities[0].(map[string]interface{})
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["processedMetadata"].(map[string]interface{})
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(100), metadata1["capacity"])
				require.Equal(t, "A", metadata1["zone"])
				require.Equal(t, float64(15), metadata1["priority"])
				// Check linkedStorages field resolver result
				linked1, ok := storage1["linkedStorages"].([]interface{})
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked1, 1, "Should return 1 linked storage (depth=1)")
				linkedStorage1, ok := linked1[0].(map[string]interface{})
				require.True(t, ok, "linked storage should be an object")
				require.Contains(t, linkedStorage1["id"], "linked-storage-1-0")
				require.Contains(t, linkedStorage1, "name")

				// Storage 2: capacity=100*2=200, zone="B" (uppercase), priority=10+10=20
				storage2, ok := entities[1].(map[string]interface{})
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				metadata2, ok := storage2["processedMetadata"].(map[string]interface{})
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(200), metadata2["capacity"])
				require.Equal(t, "B", metadata2["zone"])
				require.Equal(t, float64(20), metadata2["priority"])
				linked2, ok := storage2["linkedStorages"].([]interface{})
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked2, 1, "Should return 1 linked storage (depth=1)")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse the GraphQL schema
			schemaDoc := grpctest.MustGraphQLSchema(t)

			// Parse the GraphQL query
			queryDoc, report := astparser.ParseGraphqlDocumentString(tc.query)
			if report.HasErrors() {
				t.Fatalf("failed to parse query: %s", report.Error())
			}

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
			if err != nil {
				t.Fatalf("failed to compile proto: %v", err)
			}

			// Create the datasource
			ds, err := NewDataSource(conn, DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}
