package grpcdatasource

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

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
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")

				// Check required fields are present
				require.Contains(t, entities[0], "id")
				require.Contains(t, entities[0], "name")
				require.Contains(t, entities[1], "id")
				require.Contains(t, entities[1], "name")

				require.Len(t, entities, 4, "Should return 4 entities")

				product, ok := entities[0].(map[string]any)
				require.True(t, ok, "product should be an object")
				require.Equal(t, "1", product["id"])
				require.Equal(t, "Product 1", product["name"])

				storage, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage should be an object")
				require.Equal(t, "3", storage["id"])
				require.Equal(t, "Storage 3", storage["name"])

				product2, ok := entities[2].(map[string]any)
				require.True(t, ok, "product2 should be an object")
				require.Equal(t, "2", product2["id"])
				require.Equal(t, "Product 2", product2["name"])

				storage2, ok := entities[3].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
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
			vars: `{"variables":{
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
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Product",
					SelectionSet: "id",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")
				for index, entity := range entities {
					entity, ok := entity.(map[string]any)
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
		{
			name:  "Query Storage with filteredTagSummary (@requires + field argument)",
			query: `query($representations: [_Any!]!, $prefix: String!) { _entities(representations: $representations) { ...on Storage { __typename name filteredTagSummary(prefix: $prefix) } } }`,
			vars: `{"variables":{
				"prefix": "e",
				"representations":[
					{"__typename":"Storage","id":"1","tags":["electronics","hot-deals","books"]},
					{"__typename":"Storage","id":"2","tags":["new-arrivals","premium"]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "filteredTagSummary",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "should return 2 entities")

				// Storage 1: tags contain "electronics" which starts with "e"
				entity1, ok := entities[0].(map[string]any)
				require.True(t, ok, "entity 1 should be an object")
				require.Equal(t, "Storage", entity1["__typename"])
				require.Equal(t, "Storage 1", entity1["name"])
				require.Equal(t, "electronics", entity1["filteredTagSummary"])

				// Storage 2: no tags start with "e" → filteredTagSummary is null
				entity2, ok := entities[1].(map[string]any)
				require.True(t, ok, "entity 2 should be an object")
				require.Equal(t, "Storage", entity2["__typename"])
				require.Equal(t, "Storage 2", entity2["name"])
				require.Nil(t, entity2["filteredTagSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with multiFilteredTagSummary (@requires + two field arguments, one repeated)",
			query: `query($representations: [_Any!]!, $prefixes: [String!]!, $maxResults: Int!) { _entities(representations: $representations) { ...on Storage { __typename name multiFilteredTagSummary(prefixes: $prefixes, maxResults: $maxResults) } } }`,
			vars: `{"variables":{
				"prefixes": ["e", "h"],
				"maxResults": 2,
				"representations":[
					{"__typename":"Storage","id":"1","tags":["electronics","hot-deals","books","extra"]},
					{"__typename":"Storage","id":"2","tags":["new-arrivals","premium"]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "multiFilteredTagSummary",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "should return 2 entities")

				// Storage 1: tags "electronics" (prefix "e") and "hot-deals" (prefix "h") match; capped at maxResults=2
				entity1, ok := entities[0].(map[string]any)
				require.True(t, ok, "entity 1 should be an object")
				require.Equal(t, "Storage", entity1["__typename"])
				require.Equal(t, "Storage 1", entity1["name"])
				require.Equal(t, "electronics, hot-deals", entity1["multiFilteredTagSummary"])

				// Storage 2: no tags match any prefix → multiFilteredTagSummary is null
				entity2, ok := entities[1].(map[string]any)
				require.True(t, ok, "entity 2 should be an object")
				require.Equal(t, "Storage", entity2["__typename"])
				require.Equal(t, "Storage 2", entity2["name"])
				require.Nil(t, entity2["multiFilteredTagSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with nullableFilteredTagSummary (@requires + nullable field argument)",
			query: `query($representations: [_Any!]!, $prefix: String) { _entities(representations: $representations) { ...on Storage { __typename name nullableFilteredTagSummary(prefix: $prefix) } } }`,
			vars: `{"variables":{
				"prefix": null,
				"representations":[
					{"__typename":"Storage","id":"1","tags":["electronics","hot-deals","books"]},
					{"__typename":"Storage","id":"2","tags":[]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "nullableFilteredTagSummary",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "should return 2 entities")

				// Storage 1: prefix is null → all tags returned
				entity1, ok := entities[0].(map[string]any)
				require.True(t, ok, "entity 1 should be an object")
				require.Equal(t, "Storage", entity1["__typename"])
				require.Equal(t, "Storage 1", entity1["name"])
				require.Equal(t, "electronics, hot-deals, books", entity1["nullableFilteredTagSummary"])

				// Storage 2: no tags → nullableFilteredTagSummary is null
				entity2, ok := entities[1].(map[string]any)
				require.True(t, ok, "entity 2 should be an object")
				require.Equal(t, "Storage", entity2["__typename"])
				require.Equal(t, "Storage 2", entity2["name"])
				require.Nil(t, entity2["nullableFilteredTagSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, gjson.Valid(tc.vars))

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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
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
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]any)
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					mascot, ok := entity["mascotRecommendation"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 3, "Should return 3 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]any)
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					stockStatus, ok := entity["stockStatus"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				require.NotEmpty(t, data)

				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.NotEmpty(t, entities, "_entities should not be empty")
				require.Len(t, entities, 2, "Should return 2 entities")

				for index, entity := range entities {
					entity, ok := entity.(map[string]any)
					require.True(t, ok, "entity should be an object")
					productID := index + 1

					require.Equal(t, fmt.Sprintf("%d", productID), entity["id"])
					require.Equal(t, fmt.Sprintf("Product %d", productID), entity["name"])

					details, ok := entity["productDetails"].(map[string]any)
					require.True(t, ok, "productDetails should be an object")

					require.Contains(t, details, "id")
					require.Contains(t, details, "description")
					require.Contains(t, details["description"], "Standard details")

					// Check recommendedPet (interface)
					pet, ok := details["recommendedPet"].(map[string]any)
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
					reviewSummary, ok := details["reviewSummary"].(map[string]any)
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
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
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
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
				{"__typename":"Storage","id":"4","itemCount":400,"restockData":{"lastRestockDate":"2021-01-04"}}
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// Storage 1: itemCount=100, restockData provided -> score = 100*0.1 + 10 = 20.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, 20.0, storage1["stockHealthScore"])

				// Storage 2: itemCount=200, restockData provided -> score = 200*0.1 + 10 = 30.0
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, 30.0, storage2["stockHealthScore"])

				// Storage 3: itemCount=300, restockData provided -> score = 300*0.1 + 10 = 40.0
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "Storage 3", storage3["name"])
				require.Equal(t, 40.0, storage3["stockHealthScore"])

				// Storage 4: itemCount=400, restockData provided -> score = 400*0.1 + 10 = 50.0
				storage4, ok := entities[3].(map[string]any)
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
			name:  "Query Storage type with aliased required field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name aliasedScore: stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"lastRestockDate":"2021-01-01"}},
				{"__typename":"Storage","id":"2","itemCount":200,"restockData":{"lastRestockDate":"2021-01-02"}},
				{"__typename":"Storage","id":"3","itemCount":300,"restockData":{"lastRestockDate":"2021-01-03"}},
				{"__typename":"Storage","id":"4","itemCount":400,"restockData":{"lastRestockDate":"2021-01-04"}}
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// Storage 1: itemCount=100, restockData provided -> score = 100*0.1 + 10 = 20.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, 20.0, storage1["aliasedScore"])

				// Storage 2: itemCount=200, restockData provided -> score = 200*0.1 + 10 = 30.0
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, 30.0, storage2["aliasedScore"])

				// Storage 3: itemCount=300, restockData provided -> score = 300*0.1 + 10 = 40.0
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "Storage 3", storage3["name"])
				require.Equal(t, 40.0, storage3["aliasedScore"])

				// Storage 4: itemCount=400, restockData provided -> score = 400*0.1 + 10 = 50.0
				storage4, ok := entities[3].(map[string]any)
				require.True(t, ok, "storage4 should be an object")
				require.Equal(t, "4", storage4["id"])
				require.Equal(t, "Storage 4", storage4["name"])
				require.Equal(t, 50.0, storage4["aliasedScore"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage type with plain and aliased required field instances",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name stockHealthScore aliasedScore: stockHealthScore } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","itemCount":100,"restockData":{"lastRestockDate":"2021-01-01"}},
				{"__typename":"Storage","id":"2","itemCount":200,"restockData":{"lastRestockDate":"2021-01-02"}},
				{"__typename":"Storage","id":"3","itemCount":300,"restockData":{"lastRestockDate":"2021-01-03"}},
				{"__typename":"Storage","id":"4","itemCount":400,"restockData":{"lastRestockDate":"2021-01-04"}}
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				expectedScores := []float64{20.0, 30.0, 40.0, 50.0}
				for i, expectedScore := range expectedScores {
					storage, ok := entities[i].(map[string]any)
					require.True(t, ok, "storage%d should be an object", i+1)
					require.Equal(t, fmt.Sprintf("%d", i+1), storage["id"])
					require.Equal(t, fmt.Sprintf("Storage %d", i+1), storage["name"])
					require.Equal(t, expectedScore, storage["stockHealthScore"], "plain response key of storage%d", i+1)
					require.Equal(t, expectedScore, storage["aliasedScore"], "aliased response key of storage%d", i+1)
				}
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: itemCount=100, no restockData -> score = 100*0.1 = 10.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, 10.0, storage1["stockHealthScore"])

				// Storage 2: itemCount=500, no restockData -> score = 500*0.1 = 50.0
				storage2, ok := entities[1].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 1, "Should return 1 entity")

				// Storage 42: itemCount=1000, restockData provided -> score = 1000*0.1 + 10 = 110.0
				storage, ok := entities[0].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Just id and name, no stockHealthScore
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.NotContains(t, storage1, "stockHealthScore")

				storage2, ok := entities[1].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: tags = ["electronics", "gadgets", "sale"] -> "electronics, gadgets, sale"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "electronics, gadgets, sale", storage1["tagSummary"])

				// Storage 2: tags = ["books", "fiction"] -> "books, fiction"
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "books, fiction", storage2["tagSummary"])

				// Storage 3: tags = [] -> ""
				storage3, ok := entities[2].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: optionalTags = ["premium", "featured"] -> "premium, featured"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium, featured", storage1["optionalTagSummary"])

				// Storage 2: optionalTags = [] -> null (empty list returns nil)
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalTagSummary"])

				// Storage 3: optionalTags = null -> null
				storage3, ok := entities[2].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// Storage 1: capacity=100, zone="A" (weight=1.0) -> 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, 100.0, storage1["metadataScore"])

				// Storage 2: capacity=200, zone="B" (weight=0.8) -> 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, 160.0, storage2["metadataScore"])

				// Storage 3: capacity=300, zone="C" (weight=0.6) -> 300*0.6 = 180.0
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, 180.0, storage3["metadataScore"])

				// Storage 4: capacity=400, zone="D" (weight=0.5) -> 400*0.5 = 200.0
				storage4, ok := entities[3].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=50*2=100, zone="A" (uppercase), priority=5+10=15
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["processedMetadata"].(map[string]any)
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(100), metadata1["capacity"])
				require.Equal(t, "A", metadata1["zone"])
				require.Equal(t, float64(15), metadata1["priority"])

				// Storage 2: capacity=100*2=200, zone="B" (uppercase), priority=10+10=20
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				metadata2, ok := storage2["processedMetadata"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even): returns processed metadata
				// capacity=100*3=300, zone="x" (lowercase), priority=1
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["optionalProcessedMetadata"].(map[string]any)
				require.True(t, ok, "optionalProcessedMetadata should be an object for index 0")
				require.Equal(t, float64(300), metadata1["capacity"])
				require.Equal(t, "x", metadata1["zone"])

				// Storage 2 (index 1, odd): returns null
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalProcessedMetadata"])

				// Storage 3 (index 2, even): returns processed metadata
				// capacity=300*3=900, zone="z" (lowercase), priority=1
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				metadata3, ok := storage3["optionalProcessedMetadata"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tags = ["foo", "bar"] -> ["PROCESSED_FOO", "PROCESSED_BAR"]
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["processedTags"].([]any)
				require.True(t, ok, "processedTags should be an array")
				require.Len(t, tags1, 2)
				require.Equal(t, "PROCESSED_FOO", tags1[0])
				require.Equal(t, "PROCESSED_BAR", tags1[1])

				// Storage 2: tags = ["hello"] -> ["PROCESSED_HELLO"]
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				tags2, ok := storage2["processedTags"].([]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even with data): returns ["opt_alpha", "opt_beta"]
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["optionalProcessedTags"].([]any)
				require.True(t, ok, "optionalProcessedTags should be an array for index 0")
				require.Len(t, tags1, 2)
				require.Equal(t, "OPT_alpha", tags1[0])
				require.Equal(t, "OPT_beta", tags1[1])

				// Storage 2 (index 1, odd): returns null
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalProcessedTags"])

				// Storage 3 (index 2, even but empty): returns null (empty list returns nil)
				storage3, ok := entities[2].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: history with 2 items
				// Item 0: capacity=10*1=10, zone="HIST_A", priority=1
				// Item 1: capacity=20*2=40, zone="HIST_B", priority=2
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				history1, ok := storage1["processedMetadataHistory"].([]any)
				require.True(t, ok, "processedMetadataHistory should be an array")
				require.Len(t, history1, 2)

				item0, ok := history1[0].(map[string]any)
				require.True(t, ok, "history item should be an object")
				require.Equal(t, float64(10), item0["capacity"])
				require.Equal(t, "HIST_A", item0["zone"])
				require.Equal(t, float64(1), item0["priority"])

				item1, ok := history1[1].(map[string]any)
				require.True(t, ok, "history item should be an object")
				require.Equal(t, float64(40), item1["capacity"])
				require.Equal(t, "HIST_B", item1["zone"])
				require.Equal(t, float64(2), item1["priority"])

				// Storage 2: history with 1 item
				// Item 0: capacity=100*1=100, zone="HIST_X", priority=1
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				history2, ok := storage2["processedMetadataHistory"].([]any)
				require.True(t, ok, "processedMetadataHistory should be an array")
				require.Len(t, history2, 1)

				item2, ok := history2[0].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tagSummary = "tech, sale", metadataScore = 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "tech, sale", storage1["tagSummary"])
				require.Equal(t, 100.0, storage1["metadataScore"])

				// Storage 2: tagSummary = "books", metadataScore = 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]any)
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
		{
			name:  "Query Storage with kindSummary requiring enum field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name kindSummary } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","storageKind":"BOOK"},
				{"__typename":"Storage","id":"2","storageKind":"ELECTRONICS"},
				{"__typename":"Storage","id":"3","storageKind":"FURNITURE"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "kindSummary",
					SelectionSet: "storageKind",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: storageKind=BOOK -> "Kind: CATEGORY_KIND_BOOK"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "Kind: CATEGORY_KIND_BOOK", storage1["kindSummary"])

				// Storage 2: storageKind=ELECTRONICS -> "Kind: CATEGORY_KIND_ELECTRONICS"
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "Kind: CATEGORY_KIND_ELECTRONICS", storage2["kindSummary"])

				// Storage 3: storageKind=FURNITURE -> "Kind: CATEGORY_KIND_FURNITURE"
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "Storage 3", storage3["name"])
				require.Equal(t, "Kind: CATEGORY_KIND_FURNITURE", storage3["kindSummary"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with categoryInfoSummary requiring nested enum",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name categoryInfoSummary } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","categoryInfo":{"kind":"BOOK","name":"Fiction"}},
				{"__typename":"Storage","id":"2","categoryInfo":{"kind":"ELECTRONICS","name":"Gadgets"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "categoryInfoSummary",
					SelectionSet: "categoryInfo { kind name }",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: categoryInfo={kind:BOOK, name:"Fiction"} -> "Fiction (CATEGORY_KIND_BOOK)"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "Fiction (CATEGORY_KIND_BOOK)", storage1["categoryInfoSummary"])

				// Storage 2: categoryInfo={kind:ELECTRONICS, name:"Gadgets"} -> "Gadgets (CATEGORY_KIND_ELECTRONICS)"
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "Gadgets (CATEGORY_KIND_ELECTRONICS)", storage2["categoryInfoSummary"])
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
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
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: tags = ["electronics", "gadgets", "sale"] -> "electronics, gadgets, sale"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "electronics, gadgets, sale", storage1["tagSummary"])
				// Check storageStatus field resolver result
				status1, ok := storage1["storageStatus"].(map[string]any)
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status1, "message")
				require.Contains(t, status1["message"], "is healthy")

				// Storage 2: tags = ["books", "fiction"] -> "books, fiction"
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "books, fiction", storage2["tagSummary"])
				status2, ok := storage2["storageStatus"].(map[string]any)
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status2, "message")

				// Storage 3: tags = [] -> ""
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "", storage3["tagSummary"])
				status3, ok := storage3["storageStatus"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=100, zone="A" (weight=1.0) -> 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, 100.0, storage1["metadataScore"])
				// Check linkedStorages field resolver result
				linked1, ok := storage1["linkedStorages"].([]any)
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked1, 2, "Should return 2 linked storages (depth=2)")
				for i, linked := range linked1 {
					linkedStorage, ok := linked.(map[string]any)
					require.True(t, ok, "linked storage should be an object")
					require.Contains(t, linkedStorage["id"], fmt.Sprintf("linked-storage-1-%d", i))
					require.Contains(t, linkedStorage, "name")
				}

				// Storage 2: capacity=200, zone="B" (weight=0.8) -> 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, 160.0, storage2["metadataScore"])
				linked2, ok := storage2["linkedStorages"].([]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1: optionalTags = ["premium", "featured"] -> "premium, featured"
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium, featured", storage1["optionalTagSummary"])
				// Check nearbyStorages field resolver result with radius=3
				nearby1, ok := storage1["nearbyStorages"].([]any)
				require.True(t, ok, "nearbyStorages should be an array")
				require.Len(t, nearby1, 3, "Should return 3 nearby storages (radius=3)")

				// Storage 2: optionalTags = [] -> null (empty list returns nil)
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Nil(t, storage2["optionalTagSummary"])
				nearby2, ok := storage2["nearbyStorages"].([]any)
				require.True(t, ok, "nearbyStorages should be an array")
				require.Len(t, nearby2, 3, "Should return 3 nearby storages (radius=3)")

				// Storage 3: optionalTags = null -> null
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalTagSummary"])
				nearby3, ok := storage3["nearbyStorages"].([]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 4, "Should return 4 entities")

				// When radius is null, the mock service behavior is:
				// - Even indices (0, 2): return empty list
				// - Odd indices (1, 3): return null

				// Storage 1 (index 0, even): nearbyStorages should be empty list
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "premium", storage1["optionalTagSummary"])
				nearby1, ok := storage1["nearbyStorages"].([]any)
				require.True(t, ok, "nearbyStorages should be an empty array for even index")
				require.Len(t, nearby1, 0, "Should return empty list for index 0 when radius is null")

				// Storage 2 (index 1, odd): nearbyStorages should be null
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "featured", storage2["optionalTagSummary"])
				require.Nil(t, storage2["nearbyStorages"], "nearbyStorages should be null for odd index")

				// Storage 3 (index 2, even): nearbyStorages should be empty list
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Equal(t, "sale", storage3["optionalTagSummary"])
				nearby3, ok := storage3["nearbyStorages"].([]any)
				require.True(t, ok, "nearbyStorages should be an empty array for even index")
				require.Len(t, nearby3, 0, "Should return empty list for index 2 when radius is null")

				// Storage 4 (index 3, odd): nearbyStorages should be null
				storage4, ok := entities[3].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: tagSummary = "tech, sale", metadataScore = 100*1.0 = 100.0
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "tech, sale", storage1["tagSummary"])
				require.Equal(t, 100.0, storage1["metadataScore"])
				// Check storageStatus field resolver result
				status1, ok := storage1["storageStatus"].(map[string]any)
				require.True(t, ok, "storageStatus should be an object")
				require.Contains(t, status1, "message")
				require.Contains(t, status1["message"], "is healthy")
				require.Contains(t, status1, "timestamp")

				// Storage 2: tagSummary = "books", metadataScore = 200*0.8 = 160.0
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "books", storage2["tagSummary"])
				require.Equal(t, 160.0, storage2["metadataScore"])
				status2, ok := storage2["storageStatus"].(map[string]any)
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
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: capacity=50*2=100, zone="A" (uppercase), priority=5+10=15
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "Storage", storage1["__typename"])
				require.Equal(t, "1", storage1["id"])
				metadata1, ok := storage1["processedMetadata"].(map[string]any)
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(100), metadata1["capacity"])
				require.Equal(t, "A", metadata1["zone"])
				require.Equal(t, float64(15), metadata1["priority"])
				// Check linkedStorages field resolver result
				linked1, ok := storage1["linkedStorages"].([]any)
				require.True(t, ok, "linkedStorages should be an array")
				require.Len(t, linked1, 1, "Should return 1 linked storage (depth=1)")
				linkedStorage1, ok := linked1[0].(map[string]any)
				require.True(t, ok, "linked storage should be an object")
				require.Contains(t, linkedStorage1["id"], "linked-storage-1-0")
				require.Contains(t, linkedStorage1, "name")

				// Storage 2: capacity=100*2=200, zone="B" (uppercase), priority=10+10=20
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				metadata2, ok := storage2["processedMetadata"].(map[string]any)
				require.True(t, ok, "processedMetadata should be an object")
				require.Equal(t, float64(200), metadata2["capacity"])
				require.Equal(t, "B", metadata2["zone"])
				require.Equal(t, float64(20), metadata2["priority"])
				linked2, ok := storage2["linkedStorages"].([]any)
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

func Test_DataSource_Load_WithEntity_Calls_And_Requires_AbstractTypes(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			name:  "Query Storage with itemInfo requiring interface type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name itemInfo } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","primaryItem":{"__typename":"PalletItem","name":"Heavy Pallet","palletCount":10}},
				{"__typename":"Storage","id":"2","primaryItem":{"__typename":"ContainerItem","name":"Steel Container","containerSize":"40ft"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "itemInfo",
					SelectionSet: `primaryItem { ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } }`,
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2)

				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "Pallet: Heavy Pallet (count: 10)", storage1["itemInfo"])

				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "Container: Steel Container (size: 40ft)", storage2["itemInfo"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with operationReport requiring union type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name operationReport } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","lastStorageOperation":{"__typename":"StorageSuccess","message":"Item stored","completedAt":"2024-01-15T10:30:00Z"}},
				{"__typename":"Storage","id":"2","lastStorageOperation":{"__typename":"StorageFailure","message":"Storage full","errorCode":"ERR_FULL"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "operationReport",
					SelectionSet: `lastStorageOperation { ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } }`,
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2)

				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])
				require.Equal(t, "Success: Item stored at 2024-01-15T10:30:00Z", storage1["operationReport"])

				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])
				require.Equal(t, "Failure: Storage full (code: ERR_FULL)", storage2["operationReport"])
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

// Test_DataSource_Load_WithEntity_Calls_And_Requires_AbstractReturnTypes covers @requires
// fields whose *return* type is abstract (interface or union), as opposed to
// Test_DataSource_Load_WithEntity_Calls_And_Requires_AbstractTypes which covers abstract
// types appearing in the @requires selection set.
//
// The concrete member must be resolved from the protobuf oneof, so that __typename reports
// the concrete type and only the matching inline fragment's fields are returned.
func Test_DataSource_Load_WithEntity_Calls_And_Requires_AbstractReturnTypes(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	// entities asserts the number of returned entities and returns them as objects.
	entities := func(t *testing.T, data map[string]any, expectedLen int) []map[string]any {
		t.Helper()
		rawEntities, ok := data["_entities"].([]any)
		require.True(t, ok, "_entities should be an array")
		require.Len(t, rawEntities, expectedLen)

		result := make([]map[string]any, 0, len(rawEntities))
		for i, rawEntity := range rawEntities {
			entity, ok := rawEntity.(map[string]any)
			require.True(t, ok, "entity %d should be an object", i)
			result = append(result, entity)
		}

		return result
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			name:  "Query Storage with recommendedItem returning an interface type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id name recommendedItem { __typename ... on PalletItem { name palletCount } ... on ContainerItem { name containerSize } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":200,"zone":"A"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":50,"zone":"B"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "recommendedItem",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 2)

				// capacity 200 > 100 -> PalletItem, palletCount = capacity / 10
				storage1 := storages[0]
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "Storage 1", storage1["name"])

				item1, ok := storage1["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")
				require.Equal(t, "PalletItem", item1["__typename"])
				require.Equal(t, "Pallet for zone A", item1["name"])
				require.Equal(t, float64(20), item1["palletCount"])
				require.NotContains(t, item1, "containerSize", "ContainerItem fields must not leak into a PalletItem")

				// capacity 50 <= 100 -> ContainerItem, containerSize = "<capacity>L"
				storage2 := storages[1]
				require.Equal(t, "2", storage2["id"])
				require.Equal(t, "Storage 2", storage2["name"])

				item2, ok := storage2["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")
				require.Equal(t, "ContainerItem", item2["__typename"])
				require.Equal(t, "Container for zone B", item2["name"])
				require.Equal(t, "50L", item2["containerSize"])
				require.NotContains(t, item2, "palletCount", "PalletItem fields must not leak into a ContainerItem")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with recommendedItem selecting interface fields directly alongside fragments",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id recommendedItem { __typename id name weight ... on PalletItem { palletCount specs { name dimensions { length width } } } ... on ContainerItem { containerSize } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":200,"zone":"A"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":50,"zone":"B"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "recommendedItem",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 2)

				storage1 := storages[0]
				item1, ok := storage1["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")

				// Fields selected on the interface itself resolve for the concrete member
				require.Equal(t, "PalletItem", item1["__typename"])
				require.Equal(t, "pallet-A-200", item1["id"])
				require.Equal(t, "Pallet for zone A", item1["name"])
				require.Equal(t, float64(250), item1["weight"])
				require.Equal(t, float64(20), item1["palletCount"])

				// Concrete nesting inside the fragment resolves too
				specs, ok := item1["specs"].(map[string]any)
				require.True(t, ok, "specs should be an object")
				require.Equal(t, "Pallet for zone A specs", specs["name"])
				dimensions, ok := specs["dimensions"].(map[string]any)
				require.True(t, ok, "dimensions should be an object")
				require.Equal(t, float64(120), dimensions["length"])
				require.Equal(t, float64(80), dimensions["width"])
				require.NotContains(t, dimensions, "height", "unselected fields must not be returned")

				storage2 := storages[1]
				item2, ok := storage2["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")
				require.Equal(t, "ContainerItem", item2["__typename"])
				require.Equal(t, "container-B-50", item2["id"])
				require.Equal(t, "Container for zone B", item2["name"])
				require.Equal(t, 22.5, item2["weight"])
				require.Equal(t, "50L", item2["containerSize"])
				require.NotContains(t, item2, "specs", "PalletItem fragment fields must not leak into a ContainerItem")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with recommendedItem returning an interface type containing a nested interface type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id recommendedItem { __typename ... on PalletItem { name palletCount handler { name assignedItem { __typename ... on ContainerItem { name containerSize } ... on PalletItem { name palletCount } } } } ... on ContainerItem { name containerSize handler { name assignedItem { __typename ... on ContainerItem { name containerSize } ... on PalletItem { name palletCount } } } } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadata":{"capacity":200,"zone":"A"}},
				{"__typename":"Storage","id":"2","metadata":{"capacity":50,"zone":"B"}}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "recommendedItem",
					SelectionSet: "metadata { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 2)

				// PalletItem -> handler -> assignedItem resolves to a ContainerItem
				item1, ok := storages[0]["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")
				require.Equal(t, "PalletItem", item1["__typename"])
				require.Equal(t, "Pallet for zone A", item1["name"])

				handler1, ok := item1["handler"].(map[string]any)
				require.True(t, ok, "handler should be an object")
				require.Equal(t, "Handler for Pallet for zone A", handler1["name"])

				assigned1, ok := handler1["assignedItem"].(map[string]any)
				require.True(t, ok, "assignedItem should be an object")
				require.Equal(t, "ContainerItem", assigned1["__typename"])
				require.Equal(t, "Pallet for zone A assigned container", assigned1["name"])
				require.Equal(t, "20ft", assigned1["containerSize"])
				require.NotContains(t, assigned1, "palletCount", "the nested abstract type must resolve its own concrete member")

				// ContainerItem -> handler -> assignedItem resolves to a PalletItem
				item2, ok := storages[1]["recommendedItem"].(map[string]any)
				require.True(t, ok, "recommendedItem should be an object")
				require.Equal(t, "ContainerItem", item2["__typename"])
				require.Equal(t, "Container for zone B", item2["name"])

				handler2, ok := item2["handler"].(map[string]any)
				require.True(t, ok, "handler should be an object")
				require.Equal(t, "Handler for Container for zone B", handler2["name"])

				assigned2, ok := handler2["assignedItem"].(map[string]any)
				require.True(t, ok, "assignedItem should be an object")
				require.Equal(t, "PalletItem", assigned2["__typename"])
				require.Equal(t, "Container for zone B assigned pallet", assigned2["name"])
				require.Equal(t, float64(7), assigned2["palletCount"])
				require.NotContains(t, assigned2, "containerSize", "the nested abstract type must resolve its own concrete member")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with recommendedItems returning a list of interfaces containing nested interface types",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id recommendedItems { __typename ... on PalletItem { name handler { assignedItem { __typename ... on ContainerItem { name containerSize } } } } ... on ContainerItem { name handler { assignedItem { __typename ... on PalletItem { name palletCount } } } } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["alpha","beta"]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "recommendedItems",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storage1 := entities(t, data, 1)[0]
				items, ok := storage1["recommendedItems"].([]any)
				require.True(t, ok, "recommendedItems should be an array")
				require.Len(t, items, 2)

				// Each list element resolves its own concrete member and its own nested abstract type
				item0, ok := items[0].(map[string]any)
				require.True(t, ok, "item 0 should be an object")
				require.Equal(t, "PalletItem", item0["__typename"])
				require.Equal(t, "Pallet alpha", item0["name"])
				handler0, ok := item0["handler"].(map[string]any)
				require.True(t, ok, "handler should be an object")
				assigned0, ok := handler0["assignedItem"].(map[string]any)
				require.True(t, ok, "assignedItem should be an object")
				require.Equal(t, "ContainerItem", assigned0["__typename"])
				require.Equal(t, "Pallet alpha assigned container", assigned0["name"])
				require.Equal(t, "20ft", assigned0["containerSize"])

				item1, ok := items[1].(map[string]any)
				require.True(t, ok, "item 1 should be an object")
				require.Equal(t, "ContainerItem", item1["__typename"])
				require.Equal(t, "Container beta", item1["name"])
				handler1, ok := item1["handler"].(map[string]any)
				require.True(t, ok, "handler should be an object")
				assigned1, ok := handler1["assignedItem"].(map[string]any)
				require.True(t, ok, "assignedItem should be an object")
				require.Equal(t, "PalletItem", assigned1["__typename"])
				require.Equal(t, "Container beta assigned pallet", assigned1["name"])
				require.Equal(t, float64(7), assigned1["palletCount"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with recommendedItems returning a list of an interface type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id recommendedItems { __typename name ... on PalletItem { palletCount } ... on ContainerItem { containerSize } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["alpha","beta","gamma"]},
				{"__typename":"Storage","id":"2","tags":[]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "recommendedItems",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 2)

				// One item per tag, alternating PalletItem / ContainerItem by index
				storage1 := storages[0]
				items, ok := storage1["recommendedItems"].([]any)
				require.True(t, ok, "recommendedItems should be an array")
				require.Len(t, items, 3)

				item0, ok := items[0].(map[string]any)
				require.True(t, ok, "item 0 should be an object")
				require.Equal(t, "PalletItem", item0["__typename"])
				require.Equal(t, "Pallet alpha", item0["name"])
				require.Equal(t, float64(1), item0["palletCount"])
				require.NotContains(t, item0, "containerSize")

				item1, ok := items[1].(map[string]any)
				require.True(t, ok, "item 1 should be an object")
				require.Equal(t, "ContainerItem", item1["__typename"])
				require.Equal(t, "Container beta", item1["name"])
				require.Equal(t, "BETA", item1["containerSize"])
				require.NotContains(t, item1, "palletCount")

				item2, ok := items[2].(map[string]any)
				require.True(t, ok, "item 2 should be an object")
				require.Equal(t, "PalletItem", item2["__typename"])
				require.Equal(t, "Pallet gamma", item2["name"])
				require.Equal(t, float64(3), item2["palletCount"])

				// No tags -> empty list, not null
				storage2 := storages[1]
				emptyItems, ok := storage2["recommendedItems"].([]any)
				require.True(t, ok, "recommendedItems should be an array")
				require.Empty(t, emptyItems)
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with latestOperation returning a union type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id latestOperation { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","storageKind":"ELECTRONICS"},
				{"__typename":"Storage","id":"2","storageKind":"OTHER"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "latestOperation",
					SelectionSet: "storageKind",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 2)

				// Known kind -> StorageSuccess
				storage1 := storages[0]
				operation1, ok := storage1["latestOperation"].(map[string]any)
				require.True(t, ok, "latestOperation should be an object")
				require.Equal(t, "StorageSuccess", operation1["__typename"])
				require.Equal(t, "Operation completed for CATEGORY_KIND_ELECTRONICS", operation1["message"])
				require.Equal(t, "2024-01-01T00:00:00Z", operation1["completedAt"])
				require.NotContains(t, operation1, "errorCode", "StorageFailure fields must not leak into a StorageSuccess")

				// OTHER -> StorageFailure
				storage2 := storages[1]
				operation2, ok := storage2["latestOperation"].(map[string]any)
				require.True(t, ok, "latestOperation should be an object")
				require.Equal(t, "StorageFailure", operation2["__typename"])
				require.Equal(t, "Operation failed for CATEGORY_KIND_OTHER", operation2["message"])
				require.Equal(t, "UNSUPPORTED_KIND", operation2["errorCode"])
				require.NotContains(t, operation2, "completedAt", "StorageSuccess fields must not leak into a StorageFailure")
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with optionalLatestOperation returning a nullable union type",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalLatestOperation { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { errorCode } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","optionalTags":["a"]},
				{"__typename":"Storage","id":"2","optionalTags":["a","b"]},
				{"__typename":"Storage","id":"3","optionalTags":null}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalLatestOperation",
					SelectionSet: "optionalTags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storages := entities(t, data, 3)

				// Odd tag count -> StorageSuccess
				storage1 := storages[0]
				operation1, ok := storage1["optionalLatestOperation"].(map[string]any)
				require.True(t, ok, "optionalLatestOperation should be an object")
				require.Equal(t, "StorageSuccess", operation1["__typename"])
				require.Equal(t, "Operation completed for tags: a", operation1["message"])
				require.Equal(t, "2024-01-02T00:00:00Z", operation1["completedAt"])

				// Even tag count -> StorageFailure. Only errorCode is selected on this member.
				storage2 := storages[1]
				operation2, ok := storage2["optionalLatestOperation"].(map[string]any)
				require.True(t, ok, "optionalLatestOperation should be an object")
				require.Equal(t, "StorageFailure", operation2["__typename"])
				require.Equal(t, "EVEN_TAG_COUNT", operation2["errorCode"])
				require.NotContains(t, operation2, "message", "unselected fragment fields must not be returned")

				// No tags -> null
				storage3 := storages[2]
				require.Contains(t, storage3, "optionalLatestOperation")
				require.Nil(t, storage3["optionalLatestOperation"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with abstract return types combined with a scalar required field",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id tagSummary recommendedItems { __typename name } latestOperation { __typename ... on StorageSuccess { message } ... on StorageFailure { errorCode } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["alpha","beta"],"storageKind":"BOOK"}
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
					FieldName:    "recommendedItems",
					SelectionSet: "tags",
				},
				{
					TypeName:     "Storage",
					FieldName:    "latestOperation",
					SelectionSet: "storageKind",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				storage1 := entities(t, data, 1)[0]
				require.Equal(t, "1", storage1["id"])
				require.Equal(t, "alpha, beta", storage1["tagSummary"])

				// __typename alone is a valid selection on an abstract type
				items, ok := storage1["recommendedItems"].([]any)
				require.True(t, ok, "recommendedItems should be an array")
				require.Len(t, items, 2)

				item0, ok := items[0].(map[string]any)
				require.True(t, ok, "item 0 should be an object")
				require.Equal(t, "PalletItem", item0["__typename"])
				require.Equal(t, "Pallet alpha", item0["name"])

				item1, ok := items[1].(map[string]any)
				require.True(t, ok, "item 1 should be an object")
				require.Equal(t, "ContainerItem", item1["__typename"])
				require.Equal(t, "Container beta", item1["name"])

				operation, ok := storage1["latestOperation"].(map[string]any)
				require.True(t, ok, "latestOperation should be an object")
				require.Equal(t, "StorageSuccess", operation["__typename"])
				require.Equal(t, "Operation completed for CATEGORY_KIND_BOOK", operation["message"])
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}

// Test_DataSource_Load_WithEntity_Calls_And_Requires_NullableLists exercises @requires fields whose
// return type, field argument, or required selection set is a nullable list. Nullable lists are
// carried in a generated ListOfX wrapper message, so these cases verify that the wrapper survives
// the round trip and that a null list stays distinguishable from an empty one.
func Test_DataSource_Load_WithEntity_Calls_And_Requires_NullableLists(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	type graphqlError struct {
		Message string `json:"message"`
	}
	type graphqlResponse struct {
		Data   map[string]any `json:"data"`
		Errors []graphqlError `json:"errors,omitempty"`
	}

	testCases := []struct {
		name              string
		query             string
		vars              string
		federationConfigs plan.FederationFieldConfigurations
		validate          func(t *testing.T, data map[string]any)
		validateError     func(t *testing.T, errData []graphqlError)
	}{
		{
			/*
				optionalProcessedMetadataHistory: [StorageMetadata!] @requires(fields: "metadataHistory { capacity zone }")

				The result is wrapped in ListOfStorageMetadata.
			*/
			name:  "Query Storage with optionalProcessedMetadataHistory returning a nullable list of objects",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalProcessedMetadataHistory { capacity zone priority } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","metadataHistory":[{"capacity":10,"zone":"A"},{"capacity":20,"zone":"B"}]},
				{"__typename":"Storage","id":"2","metadataHistory":[{"capacity":100,"zone":"X"}]},
				{"__typename":"Storage","id":"3","metadataHistory":[]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalProcessedMetadataHistory",
					SelectionSet: "metadataHistory { capacity zone }",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even, non-empty history): populated list
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				history1, ok := storage1["optionalProcessedMetadataHistory"].([]any)
				require.True(t, ok, "optionalProcessedMetadataHistory should be an array")
				require.Len(t, history1, 2)
				entry1, ok := history1[0].(map[string]any)
				require.True(t, ok, "history entry should be an object")
				require.Equal(t, float64(10), entry1["capacity"])
				require.Equal(t, "OPT_HIST_A", entry1["zone"])
				require.Equal(t, float64(1), entry1["priority"])
				entry2, ok := history1[1].(map[string]any)
				require.True(t, ok, "history entry should be an object")
				require.Equal(t, float64(40), entry2["capacity"])
				require.Equal(t, "OPT_HIST_B", entry2["zone"])
				require.Equal(t, float64(2), entry2["priority"])

				// Storage 2 (index 1, odd): empty but non-null list
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				history2, ok := storage2["optionalProcessedMetadataHistory"].([]any)
				require.True(t, ok, "optionalProcessedMetadataHistory should be an empty array, not null")
				require.Empty(t, history2)

				// Storage 3 (empty history): null list
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalProcessedMetadataHistory"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			/*
				optionalRecommendedItems: [StorageItem!] @requires(fields: "tags")

				The result is wrapped in ListOfStorageItem, the item type being an interface.
			*/
			name:  "Query Storage with optionalRecommendedItems returning a nullable list of interfaces",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalRecommendedItems { __typename id name ... on PalletItem { palletCount } ... on ContainerItem { containerSize } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","tags":["alpha","beta"]},
				{"__typename":"Storage","id":"2","tags":["gamma"]},
				{"__typename":"Storage","id":"3","tags":[]}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalRecommendedItems",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (index 0, even, tags present): one item per tag, alternating concrete types
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				items1, ok := storage1["optionalRecommendedItems"].([]any)
				require.True(t, ok, "optionalRecommendedItems should be an array")
				require.Len(t, items1, 2)

				pallet, ok := items1[0].(map[string]any)
				require.True(t, ok, "first item should be an object")
				require.Equal(t, "PalletItem", pallet["__typename"])
				require.Equal(t, "opt-pallet-alpha", pallet["id"])
				require.Equal(t, "Optional pallet alpha", pallet["name"])
				require.Equal(t, float64(1), pallet["palletCount"])
				require.NotContains(t, pallet, "containerSize")

				container, ok := items1[1].(map[string]any)
				require.True(t, ok, "second item should be an object")
				require.Equal(t, "ContainerItem", container["__typename"])
				require.Equal(t, "opt-container-beta", container["id"])
				require.Equal(t, "Optional container beta", container["name"])
				require.Equal(t, "BETA", container["containerSize"])
				require.NotContains(t, container, "palletCount")

				// Storage 2 (index 1, odd): empty but non-null list
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				items2, ok := storage2["optionalRecommendedItems"].([]any)
				require.True(t, ok, "optionalRecommendedItems should be an empty array, not null")
				require.Empty(t, items2)

				// Storage 3 (no tags): null list
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalRecommendedItems"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			/*
				optionalOperationHistory: [StorageOperationResult!] @requires(fields: "storageKind")

				The result is wrapped in ListOfStorageOperationResult, the item type being a union.
			*/
			name:  "Query Storage with optionalOperationHistory returning a nullable list of unions",
			query: `query($representations: [_Any!]!) { _entities(representations: $representations) { ...on Storage { id optionalOperationHistory { __typename ... on StorageSuccess { message completedAt } ... on StorageFailure { message errorCode } } } } }`,
			vars: `{"variables":{"representations":[
				{"__typename":"Storage","id":"1","storageKind":"BOOK"},
				{"__typename":"Storage","id":"2","storageKind":"FURNITURE"},
				{"__typename":"Storage","id":"3","storageKind":"OTHER"}
			]}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "optionalOperationHistory",
					SelectionSet: "storageKind",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 3, "Should return 3 entities")

				// Storage 1 (BOOK): a success followed by a failure
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				history1, ok := storage1["optionalOperationHistory"].([]any)
				require.True(t, ok, "optionalOperationHistory should be an array")
				require.Len(t, history1, 2)

				success, ok := history1[0].(map[string]any)
				require.True(t, ok, "first entry should be an object")
				require.Equal(t, "StorageSuccess", success["__typename"])
				require.Equal(t, "History entry completed for CATEGORY_KIND_BOOK", success["message"])
				require.Equal(t, "2024-01-03T00:00:00Z", success["completedAt"])
				require.NotContains(t, success, "errorCode")

				failure, ok := history1[1].(map[string]any)
				require.True(t, ok, "second entry should be an object")
				require.Equal(t, "StorageFailure", failure["__typename"])
				require.Equal(t, "History entry failed for CATEGORY_KIND_BOOK", failure["message"])
				require.Equal(t, "HISTORIC_FAILURE", failure["errorCode"])
				require.NotContains(t, failure, "completedAt")

				// Storage 2 (FURNITURE): empty but non-null list
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				history2, ok := storage2["optionalOperationHistory"].([]any)
				require.True(t, ok, "optionalOperationHistory should be an empty array, not null")
				require.Empty(t, history2)

				// Storage 3 (OTHER): null list
				storage3, ok := entities[2].(map[string]any)
				require.True(t, ok, "storage3 should be an object")
				require.Equal(t, "3", storage3["id"])
				require.Nil(t, storage3["optionalOperationHistory"])
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			/*
				tagsByLengths(lengths: [Int!]): [String!] @requires(fields: "tags")

				Both the field argument and the result are wrapped (ListOfInt and ListOfString).
			*/
			name:  "Query Storage with tagsByLengths passing a populated nullable list argument",
			query: `query($representations: [_Any!]!, $lengths: [Int!]) { _entities(representations: $representations) { ...on Storage { id tagsByLengths(lengths: $lengths) } } }`,
			vars: `{"variables":{
				"lengths": [4, 5],
				"representations":[
					{"__typename":"Storage","id":"1","tags":["book","chair","electronics"]},
					{"__typename":"Storage","id":"2","tags":["electronics"]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagsByLengths",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 2, "Should return 2 entities")

				// Storage 1: "book" (4) and "chair" (5) match, "electronics" (11) does not
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["tagsByLengths"].([]any)
				require.True(t, ok, "tagsByLengths should be an array")
				require.Equal(t, []any{"book", "chair"}, tags1)

				// Storage 2: nothing matches -> empty but non-null list
				storage2, ok := entities[1].(map[string]any)
				require.True(t, ok, "storage2 should be an object")
				require.Equal(t, "2", storage2["id"])
				tags2, ok := storage2["tagsByLengths"].([]any)
				require.True(t, ok, "tagsByLengths should be an empty array, not null")
				require.Empty(t, tags2)
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with tagsByLengths passing an empty nullable list argument",
			query: `query($representations: [_Any!]!, $lengths: [Int!]) { _entities(representations: $representations) { ...on Storage { id tagsByLengths(lengths: $lengths) } } }`,
			vars: `{"variables":{
				"lengths": [],
				"representations":[
					{"__typename":"Storage","id":"1","tags":["book","chair"]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagsByLengths",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 1, "Should return 1 entity")

				// An empty argument list is not a null one: the mock still returns a list.
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				tags1, ok := storage1["tagsByLengths"].([]any)
				require.True(t, ok, "tagsByLengths should be an empty array, not null")
				require.Empty(t, tags1)
			},
			validateError: func(t *testing.T, errorData []graphqlError) {
				require.Empty(t, errorData)
			},
		},
		{
			name:  "Query Storage with tagsByLengths passing a null nullable list argument",
			query: `query($representations: [_Any!]!, $lengths: [Int!]) { _entities(representations: $representations) { ...on Storage { id tagsByLengths(lengths: $lengths) } } }`,
			vars: `{"variables":{
				"lengths": null,
				"representations":[
					{"__typename":"Storage","id":"1","tags":["book","chair"]}
				]
			}}`,
			federationConfigs: plan.FederationFieldConfigurations{
				{
					TypeName:     "Storage",
					SelectionSet: "id",
				},
				{
					TypeName:     "Storage",
					FieldName:    "tagsByLengths",
					SelectionSet: "tags",
				},
			},
			validate: func(t *testing.T, data map[string]any) {
				entities, ok := data["_entities"].([]any)
				require.True(t, ok, "_entities should be an array")
				require.Len(t, entities, 1, "Should return 1 entity")

				// A null argument list reaches the service as null, so the result is null too.
				storage1, ok := entities[0].(map[string]any)
				require.True(t, ok, "storage1 should be an object")
				require.Equal(t, "1", storage1["id"])
				require.Nil(t, storage1["tagsByLengths"])
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
			ds, err := NewDataSource(NewGRPCTransport(conn), DataSourceConfig{
				Operation:         &queryDoc,
				Definition:        &schemaDoc,
				SubgraphName:      "Products",
				Mapping:           testMapping(),
				Compiler:          compiler,
				FederationConfigs: tc.federationConfigs,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			data, err := ds.Load(context.Background(), nil, []byte(input))
			require.NoError(t, err)

			// Parse the response
			var resp graphqlResponse

			err = json.Unmarshal(data, &resp)
			require.NoError(t, err, "Failed to unmarshal response")

			tc.validate(t, resp.Data)
			tc.validateError(t, resp.Errors)
		})
	}
}
