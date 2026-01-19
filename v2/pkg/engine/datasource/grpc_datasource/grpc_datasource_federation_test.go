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
