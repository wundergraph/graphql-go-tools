package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/productv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/encoding/protojson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// mockInterface provides a simple implementation of grpc.ClientConnInterface for testing
type mockInterface struct {
}

func (m mockInterface) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	fmt.Println(method, args, reply)

	msg, ok := reply.(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("reply is not a dynamicpb.Message")
	}

	// Based on the method name, populate the response with appropriate test data
	if strings.HasSuffix(method, "QueryComplexFilterType") {
		// Populate the response with test data using protojson.Unmarshal
		responseJSON := []byte(`{"complexFilterType":[{"id":"test-id-123", "name":"Test Product"}]}`)
		err := protojson.Unmarshal(responseJSON, msg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m mockInterface) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("implement me")
}

var _ grpc.ClientConnInterface = (*mockInterface)(nil)

func setupTestGRPCServer(t *testing.T) (conn *grpc.ClientConn, cleanup func()) {
	t.Helper()

	// Set up the bufconn listener
	lis := bufconn.Listen(1024 * 1024)

	// Create a new gRPC server
	server := grpc.NewServer()

	// Register our mock service implementation
	mockService := &grpctest.MockService{}
	productv1.RegisterProductServiceServer(server, mockService)

	// Start the server in a goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	// Create a buffer-based dialer
	bufDialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	// Connect using bufconn dialer
	// see https://github.com/grpc/grpc-go/issues/7091
	// nolint: staticcheck
	conn, err := grpc.Dial(
		"bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(bufDialer),
		grpc.WithLocalDNSResolution(),
	)
	require.NoError(t, err)

	cleanup = func() {
		conn.Close()
		server.Stop()
		lis.Close()
	}

	return conn, cleanup
}

// Test_DataSource_Load tests the datasource.Load method with a mock gRPC interface
func Test_DataSource_Load(t *testing.T) {
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, queryReport := astparser.ParseGraphqlDocumentString(query)
	if queryReport.HasErrors() {
		t.Fatalf("failed to parse query: %s", queryReport.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	mi := mockInterface{}
	ds, err := NewDataSource(mi, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping: &GRPCMapping{
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
					},
				},
			},
		},
	})

	require.NoError(t, err)

	output := new(bytes.Buffer)

	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
	require.NoError(t, err)

	fmt.Println(output.String())
}

// Test_DataSource_Load_WithMockService tests the datasource.Load method with an actual gRPC server
// TODO update this test to not use mappings anc expect no response
func Test_DataSource_Load_WithMockService(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// 1. Set up GraphQL query and schema
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// 2. Create a datasource with the real gRPC client connection
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping: &GRPCMapping{
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
					},
				},
				"FilterType": {
					"name": {
						TargetName: "name",
					},
					"filterField1": {
						TargetName: "filter_field_1",
					},
					"filterField2": {
						TargetName: "filter_field_2",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// 3. Execute the query through our datasource
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","body":`+variables+`}`), output)
	require.NoError(t, err)

	// Print the response for debugging
	// fmt.Println(output.String())

	type response struct {
		Data struct {
			ComplexFilterType []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"complexFilterType"`
		} `json:"data"`
	}

	var resp response

	bytes := output.Bytes()
	fmt.Println(string(bytes))

	err = json.Unmarshal(bytes, &resp)
	require.NoError(t, err)

	require.Equal(t, "test-id-123", resp.Data.ComplexFilterType[0].Id)
	require.Equal(t, "Test Product", resp.Data.ComplexFilterType[0].Name)
}

func Test_DataSource_Load_WithMockService_WithResponseMapping(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// 1. Set up GraphQL query and schema
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"HARDCODED_NAME_TEST","filterField1":"value1","filterField2":"value2"}}}}`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// 2. Create a datasource with the real gRPC client connection
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
		Mapping: &GRPCMapping{
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
					},
				},
				"FilterType": {
					"name": {
						TargetName: "name",
					},
					"filterField1": {
						TargetName: "filter_field_1",
					},
					"filterField2": {
						TargetName: "filter_field_2",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	// 3. Execute the query through our datasource
	output := new(bytes.Buffer)

	// Format the input with query and variables
	inputJSON := fmt.Sprintf(`{"query":%q,"body":%s}`, query, variables)

	err = ds.Load(context.Background(), []byte(inputJSON), output)
	require.NoError(t, err)

	// Set up the correct response structure based on your GraphQL schema
	type response struct {
		Data struct {
			ComplexFilterType []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"complexFilterType"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	var resp response
	err = json.Unmarshal(output.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")

	// Check if there are any errors in the response
	if len(resp.Errors) > 0 {
		t.Fatalf("GraphQL errors: %s", resp.Errors[0].Message)
	}

	// Check if we have the expected data
	require.NotNil(t, resp.Data.ComplexFilterType, "ComplexFilterType should not be nil")
	require.NotEmpty(t, resp.Data.ComplexFilterType, "ComplexFilterType should not be empty")

	// Now we can safely access the first element and verify the hardcoded name
	require.Equal(t, "test-id-123", resp.Data.ComplexFilterType[0].Id)
	require.Equal(t, "HARDCODED_NAME_TEST", resp.Data.ComplexFilterType[0].Name)
}

// Test_DataSource_Load_WithGrpcError tests how the datasource handles gRPC errors
// and formats them as GraphQL errors in the response
func Test_DataSource_Load_WithGrpcError(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// 1. Set up the GraphQL query that will trigger the error
	query := `query UserQuery($id: ID!) { user(id: $id) { id name } }`
	variables := `{"variables":{"id":"error-user"}}`

	// 2. Parse the schema and query
	schemaDoc := grpctest.MustGraphQLSchema(t)

	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// 3. Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
	})
	require.NoError(t, err)

	// 4. Execute the query
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","body":`+variables+`}`), output)
	require.NoError(t, err, "Load should not return an error even when the gRPC call fails")

	responseJson := output.String()

	// 5. Verify the response format according to GraphQL specification
	// The response should have an "errors" array with the error message
	require.Contains(t, responseJson, "errors")
	require.Contains(t, responseJson, "user not found: error-user")

	// 6. Parse the response JSON for more detailed validation
	var response struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	err = json.Unmarshal(output.Bytes(), &response)
	require.NoError(t, err, "Failed to parse response JSON")

	// Verify there's at least one error
	require.NotEmpty(t, response.Errors, "Expected errors array to not be empty")

	// Verify the error message
	require.Contains(t, response.Errors[0].Message, "user not found: error-user")
}

func TestMarshalResponseJSON(t *testing.T) {

	// Create an execution plan that defines how to build the protobuf message
	// This plan describes how to call the LookupProductById method
	// Define the structure of the response message
	response := RPCMessage{
		Name: "LookupProductByIdResponse",
		Fields: []RPCField{
			{
				Name:     "result",
				TypeName: string(DataTypeMessage),
				Repeated: true,
				JSONPath: "_entities",
				Message: &RPCMessage{
					Name: "Product",
					Fields: []RPCField{
						{
							Name:        "__typename",
							TypeName:    string(DataTypeString),
							JSONPath:    "__typename",
							StaticValue: "Product",
						},
						{
							Name:     "id",
							TypeName: string(DataTypeString),
							JSONPath: "id",
						},
						{
							Name:     "name",
							TypeName: string(DataTypeString),
							JSONPath: "name_different",
						},
						{
							Name:     "price",
							TypeName: string(DataTypeDouble),
							JSONPath: "price_different",
						},
					},
				},
			},
		},
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	productMessageDesc := compiler.doc.MessageByName("Product").Desc
	productMessage := dynamicpb.NewMessage(productMessageDesc)
	productMessage.Set(productMessageDesc.Fields().ByName("id"), protoref.ValueOfString("123"))
	productMessage.Set(productMessageDesc.Fields().ByName("name"), protoref.ValueOfString("test"))
	productMessage.Set(productMessageDesc.Fields().ByName("price"), protoref.ValueOfFloat64(123.45))

	responseMessageDesc := compiler.doc.MessageByName("LookupProductByIdResponse").Desc
	responseMessage := dynamicpb.NewMessage(responseMessageDesc)
	responseMessage.Mutable(responseMessageDesc.Fields().ByName("result")).List().Append(protoref.ValueOfMessage(productMessage))

	ds := &DataSource{}

	arena := astjson.Arena{}
	responseJSON, err := ds.marshalResponseJSON(&arena, &response, responseMessage)
	require.NoError(t, err)
	require.Equal(t, `{"_entities":[{"__typename":"Product","id":"123","name_different":"test","price_different":123.45}]}`, responseJSON.String())
}

// Test_DataSource_Load_WithAnimalInterface tests the datasource with Animal interface types (Cat/Dog)
// using a bufconn connection to the mock service
func Test_DataSource_Load_WithAnimalInterface(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "Query random pet with only common fields",
			query: `query RandomPetQuery {
				randomPet {
					__typename
					id
					name
					kind
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				randomPet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotNil(t, randomPet, "RandomPet should not be nil")

				// Verify common fields
				require.Contains(t, randomPet, "__typename")
				require.Contains(t, randomPet, "id")
				require.Contains(t, randomPet, "name")
				require.Contains(t, randomPet, "kind")

				// Verify __typename is either Cat or Dog
				typename := randomPet["__typename"].(string)
				require.Contains(t, []string{"Cat", "Dog"}, typename, "typename should be either Cat or Dog")

				// Verify specific fields are not present since they weren't requested
				require.NotContains(t, randomPet, "meowVolume")
				require.NotContains(t, randomPet, "barkVolume")
			},
		},
		{
			name: "Query random pet with full interface fields",
			query: `query RandomPetQuery {
				randomPet {
					__typename
					id
					name
					kind
					... on Cat {
						meowVolume
					}
					... on Dog {
						barkVolume
					}
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				randomPet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotNil(t, randomPet, "RandomPet should not be nil")

				// Check if we got either a cat or dog by checking for their specific fields
				if _, hasCat := randomPet["meowVolume"]; hasCat {
					// We got a Cat response
					require.Contains(t, randomPet, "__typename")
					require.Equal(t, "Cat", randomPet["__typename"])
					require.Contains(t, randomPet, "id")
					require.Contains(t, randomPet, "name")
					require.Contains(t, randomPet, "kind")
					require.Contains(t, randomPet, "meowVolume")
				} else if _, hasDog := randomPet["barkVolume"]; hasDog {
					// We got a Dog response
					require.Contains(t, randomPet, "__typename")
					require.Equal(t, "Dog", randomPet["__typename"])
					require.Contains(t, randomPet, "id")
					require.Contains(t, randomPet, "name")
					require.Contains(t, randomPet, "kind")
					require.Contains(t, randomPet, "barkVolume")
				} else {
					t.Fatalf("Response doesn't contain either a Cat or Dog type: %v", randomPet)
				}
			},
		},
		{
			name: "Query random pet with only Cat fragment",
			query: `query RandomPetQuery {
				randomPet {
					__typename
					id
					name
					... on Cat {
						meowVolume
					}
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				randomPet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotNil(t, randomPet, "RandomPet should not be nil")

				// Common fields should always be present
				require.Contains(t, randomPet, "__typename")
				require.Contains(t, randomPet, "id")
				require.Contains(t, randomPet, "name")

				typename := randomPet["__typename"].(string)
				require.Contains(t, []string{"Cat", "Dog"}, typename, "typename should be either Cat or Dog")

				// If it's a Cat, meowVolume should be present
				if typename == "Cat" {
					require.Contains(t, randomPet, "meowVolume")
				}
				// barkVolume should never be present since it wasn't requested
				require.NotContains(t, randomPet, "barkVolume")
			},
		},
		{
			name: "Query random pet with only Animal fragment",
			query: `query RandomPetQuery {
				randomPet {
					__typename
					... on Animal {
						kind
					}
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				randomPet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotNil(t, randomPet, "RandomPet should not be nil")

				// Common fields should always be present
				require.Contains(t, randomPet, "__typename")
				require.Contains(t, randomPet, "kind")

				typename := randomPet["__typename"].(string)
				require.Contains(t, []string{"Cat", "Dog"}, typename, "typename should be either Cat or Dog")
			},
		},
		{
			name: "Query random pet with Animal and Member fragments",
			query: `query RandomPetQuery {
				randomPet {
					__typename
					... on Animal {
						id
						kind
					}
					... on Cat {
						id
						meowVolume
					}
					... on Dog {
						id
						name
						barkVolume
					}
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				randomPet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotNil(t, randomPet, "RandomPet should not be nil")

				// Common fields should always be present
				require.Contains(t, randomPet, "__typename")
				require.Contains(t, randomPet, "kind")

				typename := randomPet["__typename"].(string)
				require.Contains(t, []string{"Cat", "Dog"}, typename, "typename should be either Cat or Dog")

				switch typename {
				case "Cat":
					require.Contains(t, randomPet, "id")
					require.Contains(t, randomPet, "meowVolume")
					require.Contains(t, randomPet, "kind")
					require.NotContains(t, randomPet, "name")
					require.NotContains(t, randomPet, "barkVolume")

					require.Equal(t, "cat-1", randomPet["id"])
					require.Equal(t, "Siamese", randomPet["kind"])
				case "Dog":
					require.Contains(t, randomPet, "id")
					require.Contains(t, randomPet, "name")
					require.Contains(t, randomPet, "kind")
					require.Contains(t, randomPet, "barkVolume")
					require.NotContains(t, randomPet, "meowVolume")

					require.Equal(t, "dog-1", randomPet["id"])
					require.Equal(t, "Dalmatian", randomPet["kind"])
					require.Equal(t, "Spot", randomPet["name"])
				}
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
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Compiler:     compiler,
				Mapping:      testMapping(),
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp struct {
				Data   map[string]interface{} `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")
			require.Empty(t, resp.Errors, "Response should not contain errors")
			require.NotEmpty(t, resp.Data, "Response should contain data")

			// Run the validation function
			tc.validate(t, resp.Data)
		})
	}
}

func Test_Datasource_Load_WithUnionTypes(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name:  "Query random search result",
			query: `query { randomSearchResult { __typename ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				searchResult, ok := data["randomSearchResult"].(map[string]interface{})
				require.True(t, ok, "randomSearchResult should be an object")
				require.NotEmpty(t, searchResult, "randomSearchResult should not be empty")
				require.Contains(t, searchResult, "__typename")
				typeName := searchResult["__typename"].(string)

				switch typeName {
				case "Product":
					require.Contains(t, searchResult, "id")
					require.Contains(t, searchResult, "name")
					require.Contains(t, searchResult, "price")
					require.Equal(t, "product-random-1", searchResult["id"])
					require.Equal(t, "Random Product", searchResult["name"])
					require.Equal(t, 29.99, searchResult["price"])
				case "User":
					require.Contains(t, searchResult, "id")
					require.Contains(t, searchResult, "name")
					require.Equal(t, "user-random-1", searchResult["id"])
					require.Equal(t, "Random User", searchResult["name"])
				case "Category":
					require.Contains(t, searchResult, "id")
					require.Contains(t, searchResult, "name")
					require.Contains(t, searchResult, "kind")
					require.Equal(t, "category-random-1", searchResult["id"])
					require.Equal(t, "Random Category", searchResult["name"])
					require.Equal(t, "ELECTRONICS", searchResult["kind"])
				default:
					t.Fatalf("Unexpected __typename: %s", typeName)
				}
			},
		},
		{
			name:  "Query search with input - mixed results",
			query: `query($input: SearchInput!) { search(input: $input) { __typename ... on Product { id name price } ... on User { id name } ... on Category { id name kind } } }`,
			vars:  `{"variables":{"input":{"query":"test","limit":6}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				searchResults, ok := data["search"].([]interface{})
				require.True(t, ok, "search should be an array")
				require.NotEmpty(t, searchResults, "search should not be empty")
				require.Len(t, searchResults, 6, "should return 6 results as per limit")

				// Verify we have a mix of all three types (Product, User, Category)
				var productCount, userCount, categoryCount int
				for i, result := range searchResults {
					searchResult := result.(map[string]interface{})
					require.Contains(t, searchResult, "__typename")
					typeName := searchResult["__typename"].(string)

					switch typeName {
					case "Product":
						productCount++
						require.Contains(t, searchResult, "id")
						require.Contains(t, searchResult, "name")
						require.Contains(t, searchResult, "price")
						expectedID := fmt.Sprintf("product-search-%d", (i/3)*3+1)
						require.Equal(t, expectedID, searchResult["id"])
						require.Contains(t, searchResult["name"].(string), "Product matching 'test'")
					case "User":
						userCount++
						require.Contains(t, searchResult, "id")
						require.Contains(t, searchResult, "name")
						expectedID := fmt.Sprintf("user-search-%d", ((i-1)/3)*3+2)
						require.Equal(t, expectedID, searchResult["id"])
						require.Contains(t, searchResult["name"].(string), "User matching 'test'")
					case "Category":
						categoryCount++
						require.Contains(t, searchResult, "id")
						require.Contains(t, searchResult, "name")
						require.Contains(t, searchResult, "kind")
						expectedID := fmt.Sprintf("category-search-%d", ((i-2)/3)*3+3)
						require.Equal(t, expectedID, searchResult["id"])
						require.Contains(t, searchResult["name"].(string), "Category matching 'test'")
					default:
						t.Fatalf("Unexpected __typename: %s", typeName)
					}
				}

				// Verify we have exactly 2 of each type (cycling through Product, User, Category)
				require.Equal(t, 2, productCount, "should have 2 products")
				require.Equal(t, 2, userCount, "should have 2 users")
				require.Equal(t, 2, categoryCount, "should have 2 categories")
			},
		},
		{
			name:  "Query search with limited results",
			query: `query($input: SearchInput!) { search(input: $input) { __typename ... on Product { id name } ... on User { id name } } }`,
			vars:  `{"variables":{"input":{"query":"limited","limit":3}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				searchResults, ok := data["search"].([]interface{})
				require.True(t, ok, "search should be an array")
				require.NotEmpty(t, searchResults, "search should not be empty")
				require.Len(t, searchResults, 3, "should return 3 results as per limit")

				// Only check Product and User types since Category fragments are not selected
				for _, result := range searchResults {
					searchResult := result.(map[string]interface{})
					require.Contains(t, searchResult, "__typename")
					typeName := searchResult["__typename"].(string)

					switch typeName {
					case "Product":
						require.Contains(t, searchResult, "id")
						require.Contains(t, searchResult, "name")
						require.NotContains(t, searchResult, "price", "price should not be selected")
					case "User":
						require.Contains(t, searchResult, "id")
						require.Contains(t, searchResult, "name")
					case "Category":
						// Category should still have __typename, but won't have other fields since they weren't selected
						require.Contains(t, searchResult, "__typename")
						require.NotContains(t, searchResult, "name", "name should not be selected for Category")
						require.NotContains(t, searchResult, "kind", "kind should not be selected for Category")
					default:
						t.Fatalf("Unexpected __typename: %s", typeName)
					}
				}
			},
		},
		{
			name:  "Mutation perform action - success case",
			query: `mutation($input: ActionInput!) { performAction(input: $input) { __typename ... on ActionSuccess { message timestamp } ... on ActionError { message code } } }`,
			vars:  `{"variables":{"input":{"type":"create_user","payload":"user data"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				actionResult, ok := data["performAction"].(map[string]interface{})
				require.True(t, ok, "performAction should be an object")
				require.NotEmpty(t, actionResult, "performAction should not be empty")
				require.Contains(t, actionResult, "__typename")
				require.Equal(t, "ActionSuccess", actionResult["__typename"])

				require.Contains(t, actionResult, "message")
				require.Contains(t, actionResult, "timestamp")
				require.Equal(t, "Action 'create_user' completed successfully", actionResult["message"])
				require.Equal(t, "2024-01-01T00:00:00Z", actionResult["timestamp"])
			},
		},
		{
			name:  "Mutation perform action - validation error case",
			query: `mutation($input: ActionInput!) { performAction(input: $input) { __typename ... on ActionSuccess { message timestamp } ... on ActionError { message code } } }`,
			vars:  `{"variables":{"input":{"type":"error_action","payload":"invalid data"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				actionResult, ok := data["performAction"].(map[string]interface{})
				require.True(t, ok, "performAction should be an object")
				require.NotEmpty(t, actionResult, "performAction should not be empty")
				require.Contains(t, actionResult, "__typename")
				require.Equal(t, "ActionError", actionResult["__typename"])

				require.Contains(t, actionResult, "message")
				require.Contains(t, actionResult, "code")
				require.Equal(t, "Action failed due to validation error", actionResult["message"])
				require.Equal(t, "VALIDATION_ERROR", actionResult["code"])
			},
		},
		{
			name:  "Mutation perform action - invalid action error case",
			query: `mutation($input: ActionInput!) { performAction(input: $input) { __typename ... on ActionSuccess { message timestamp } ... on ActionError { message code } } }`,
			vars:  `{"variables":{"input":{"type":"invalid_action","payload":"test"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				actionResult, ok := data["performAction"].(map[string]interface{})
				require.True(t, ok, "performAction should be an object")
				require.NotEmpty(t, actionResult, "performAction should not be empty")
				require.Contains(t, actionResult, "__typename")
				require.Equal(t, "ActionError", actionResult["__typename"])

				require.Contains(t, actionResult, "message")
				require.Contains(t, actionResult, "code")
				require.Equal(t, "Invalid action type provided", actionResult["message"])
				require.Equal(t, "INVALID_ACTION", actionResult["code"])
			},
		},
		{
			name:  "Mutation perform action - only success fragment",
			query: `mutation($input: ActionInput!) { performAction(input: $input) { __typename ... on ActionSuccess { message timestamp } } }`,
			vars:  `{"variables":{"input":{"type":"success_only","payload":"test"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				actionResult, ok := data["performAction"].(map[string]interface{})
				require.True(t, ok, "performAction should be an object")
				require.NotEmpty(t, actionResult, "performAction should not be empty")
				require.Contains(t, actionResult, "__typename")
				require.Equal(t, "ActionSuccess", actionResult["__typename"])

				require.Contains(t, actionResult, "message")
				require.Contains(t, actionResult, "timestamp")
				require.Equal(t, "Action 'success_only' completed successfully", actionResult["message"])
				require.Equal(t, "2024-01-01T00:00:00Z", actionResult["timestamp"])
			},
		},
		{
			name:  "Mutation perform action - only error fragment",
			query: `mutation($input: ActionInput!) { performAction(input: $input) { __typename ... on ActionError { message code } } }`,
			vars:  `{"variables":{"input":{"type":"error_action","payload":"test"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				actionResult, ok := data["performAction"].(map[string]interface{})
				require.True(t, ok, "performAction should be an object")
				require.NotEmpty(t, actionResult, "performAction should not be empty")
				require.Contains(t, actionResult, "__typename")
				require.Equal(t, "ActionError", actionResult["__typename"])

				require.Contains(t, actionResult, "message")
				require.Contains(t, actionResult, "code")
				require.Equal(t, "Action failed due to validation error", actionResult["message"])
				require.Equal(t, "VALIDATION_ERROR", actionResult["code"])
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
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Mapping:      testMapping(),
				Compiler:     compiler,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp struct {
				Data   map[string]interface{} `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")
			require.Empty(t, resp.Errors, "Response should not contain errors")
			require.NotEmpty(t, resp.Data, "Response should contain data")

			// Run the validation function
			tc.validate(t, resp.Data)
		})
	}
}

// Test_DataSource_Load_WithProductQueries tests the product-related query operations
// Category queries are used to mainly focus on testing Enum values
func Test_DataSource_Load_WithCategoryQueries(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// Define test cases
	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "Query all categories",
			query: `query {
				categories {
					id
					name
					kind
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				categories, ok := data["categories"].([]interface{})
				require.True(t, ok, "categories should be an array")
				require.NotEmpty(t, categories, "categories should not be empty")

				// Check the first category
				category := categories[0].(map[string]interface{})
				require.Contains(t, category, "id")
				require.Contains(t, category, "name")
				require.Contains(t, category, "kind")
			},
		},
		{
			name: "Query categories by kind",
			query: `query($kind: CategoryKind!) {
				categoriesByKind(kind: $kind) {
					id
					name
					kind
				}
			}`,
			vars: `{"variables":{"kind":"FURNITURE"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				categories, ok := data["categoriesByKind"].([]interface{})
				require.True(t, ok, "categoriesByKind should be an array")
				require.NotEmpty(t, categories, "categoriesByKind should not be empty")

				// Check the categories are all of the requested kind
				for _, c := range categories {
					category := c.(map[string]interface{})
					require.Equal(t, "FURNITURE", category["kind"], "category should have the requested kind")
				}
			},
		},
		{
			name: "Filter categories with pagination",
			query: `query($filter: CategoryFilter!) {
				filterCategories(filter: $filter) {
					id
					name
					kind
				}
			}`,
			vars: `{"variables":{"filter":{"category":"ELECTRONICS","pagination":{"page":1,"perPage":2}}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				categories, ok := data["filterCategories"].([]interface{})
				require.True(t, ok, "filterCategories should be an array")
				require.NotEmpty(t, categories, "filterCategories should not be empty")

				// Should respect the pagination limit
				require.LessOrEqual(t, len(categories), 2, "should return at most 2 categories due to pagination")

				// Categories should have the requested kind
				for _, c := range categories {
					category := c.(map[string]interface{})
					require.Equal(t, "ELECTRONICS", category["kind"], "category should have the requested kind")
				}
			},
		},
	}

	// Run the test cases
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
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Mapping:      testMapping(),
				Compiler:     compiler,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp struct {
				Data   map[string]interface{} `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")
			require.Empty(t, resp.Errors, "Response should not contain errors")
			require.NotEmpty(t, resp.Data, "Response should contain data")

			// Run the validation function
			tc.validate(t, resp.Data)
		})
	}
}

// Test_DataSource_Load_WithTotalCalculation tests the calculation of order totals using the
// MockService implementation
func Test_DataSource_Load_WithTotalCalculation(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// Define the GraphQL query
	query := `
	query CalculateTotals($orders: [OrderInput!]!) {
		calculateTotals(orders: $orders) {
			orderId
			customerName
			totalItems
			orderLines {
				productId
				quantity
				modifiers
			}
		}
	}`

	variables := `{"variables":{"orders":[
		{"orderId":"order-1","customerName":"John Doe","lines":[
			{"productId":"product-1","quantity":3,"modifiers":["discount-10"]},
			{"productId":"product-2","quantity":2,"modifiers":["tax-20"]}
		]},
		{"orderId":"order-2","customerName":"Jane Smith","lines":[
			{"productId":"product-3","quantity":1,"modifiers":["discount-15"]},
			{"productId":"product-4","quantity":5,"modifiers":["tax-25"]}
		]}
	]}}`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      testMapping(),
		Compiler:     compiler,
	})
	require.NoError(t, err)

	// Execute the query through our datasource
	output := new(bytes.Buffer)
	input := fmt.Sprintf(`{"query":%q,"body":%s}`, query, variables)
	err = ds.Load(context.Background(), []byte(input), output)
	require.NoError(t, err)

	// Parse the response
	var resp struct {
		Data struct {
			CalculateTotals []struct {
				OrderId      string `json:"orderId"`
				CustomerName string `json:"customerName"`
				TotalItems   int    `json:"totalItems"`
				OrderLines   []struct {
					ProductId string   `json:"productId"`
					Quantity  int      `json:"quantity"`
					Modifiers []string `json:"modifiers"`
				} `json:"orderLines"`
			} `json:"calculateTotals"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	err = json.Unmarshal(output.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")
	require.Empty(t, resp.Errors, "Response should not contain errors")

	// Verify the orders were returned
	require.Len(t, resp.Data.CalculateTotals, 2, "Should return 2 orders")

	// Verify the first order
	firstOrder := resp.Data.CalculateTotals[0]
	require.Equal(t, "order-1", firstOrder.OrderId)
	require.Equal(t, "John Doe", firstOrder.CustomerName)
	require.Equal(t, 5, firstOrder.TotalItems, "First order should have 3+2=5 total items")
	require.Len(t, firstOrder.OrderLines, 2, "First order should have 2 order lines")
	require.Equal(t, "product-1", firstOrder.OrderLines[0].ProductId)
	require.Equal(t, 3, firstOrder.OrderLines[0].Quantity)
	require.Equal(t, []string{"discount-10"}, firstOrder.OrderLines[0].Modifiers)
	require.Equal(t, "product-2", firstOrder.OrderLines[1].ProductId)
	require.Equal(t, 2, firstOrder.OrderLines[1].Quantity)

	// Verify the second order
	secondOrder := resp.Data.CalculateTotals[1]
	require.Equal(t, "order-2", secondOrder.OrderId)
	require.Equal(t, "Jane Smith", secondOrder.CustomerName)
	require.Equal(t, 6, secondOrder.TotalItems, "Second order should have 1+5=6 total items")
	require.Len(t, secondOrder.OrderLines, 2, "Second order should have 2 order lines")
	require.Equal(t, "product-3", secondOrder.OrderLines[0].ProductId)
	require.Equal(t, 1, secondOrder.OrderLines[0].Quantity)
	require.Equal(t, []string{"discount-15"}, secondOrder.OrderLines[0].Modifiers)
	require.Equal(t, "product-4", secondOrder.OrderLines[1].ProductId)
	require.Equal(t, 5, secondOrder.OrderLines[1].Quantity)
	require.Equal(t, []string{"tax-25"}, secondOrder.OrderLines[1].Modifiers)
}

// Test_DataSource_Load_WithTypename tests that __typename fields are correctly included
// in the response with their static values
func Test_DataSource_Load_WithTypename(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	// Define GraphQL query that requests __typename
	query := `query UsersWithTypename { users { __typename id name } }`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      testMapping(),
		Compiler:     compiler,
	})
	require.NoError(t, err)

	// Execute the query through our datasource
	output := new(bytes.Buffer)
	input := fmt.Sprintf(`{"query":%q,"body":{}}`, query)
	err = ds.Load(context.Background(), []byte(input), output)
	require.NoError(t, err)

	// Parse the response
	var resp struct {
		Data struct {
			Users []struct {
				Typename string `json:"__typename"`
				ID       string `json:"id"`
				Name     string `json:"name"`
			} `json:"users"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	err = json.Unmarshal(output.Bytes(), &resp)
	require.NoError(t, err, "Failed to unmarshal response")
	require.Empty(t, resp.Errors, "Response should not contain errors")

	// Verify response data
	require.NotEmpty(t, resp.Data.Users, "Users array should not be empty")

	// Check that each user has the correct __typename
	for _, user := range resp.Data.Users {
		require.Equal(t, "User", user.Typename, "Each user should have __typename set to 'User'")
		require.NotEmpty(t, user.ID, "User ID should not be empty")
		require.NotEmpty(t, user.Name, "User name should not be empty")
	}
}

// Test_DataSource_Load_WithAliases tests various GraphQL alias scenarios
// with the actual gRPC service using bufconn
func Test_DataSource_Load_WithAliases(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name:  "Simple root field alias",
			query: `query { allUsers: users { id name } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				users, ok := data["allUsers"].([]interface{})
				require.True(t, ok, "allUsers should be an array")
				require.NotEmpty(t, users, "allUsers should not be empty")

				user := users[0].(map[string]interface{})
				require.Contains(t, user, "id")
				require.Contains(t, user, "name")
				require.NotEmpty(t, user["id"])
				require.NotEmpty(t, user["name"])
			},
		},
		{
			name:  "Field alias with arguments and nested field aliases",
			query: `query { specificUser: user(id: $id) { userId: id userName: name } }`,
			vars:  `{"variables": {"id": "123"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				user, ok := data["specificUser"].(map[string]interface{})
				require.True(t, ok, "specificUser should be an object")
				require.NotEmpty(t, user, "specificUser should not be empty")

				require.Contains(t, user, "userId")
				require.Contains(t, user, "userName")
				require.Equal(t, "123", user["userId"])
				require.Equal(t, "User 123", user["userName"])

				// Ensure original field names are not present
				require.NotContains(t, user, "id")
				require.NotContains(t, user, "name")
			},
		},
		{
			name:  "Multiple aliases on the same level",
			query: `query { allUsers: users { id name } allCategories: categories { id name categoryType: kind } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				// Check users alias
				users, ok := data["allUsers"].([]interface{})
				require.True(t, ok, "allUsers should be an array")
				require.NotEmpty(t, users, "allUsers should not be empty")

				// Check categories alias
				categories, ok := data["allCategories"].([]interface{})
				require.True(t, ok, "allCategories should be an array")
				require.NotEmpty(t, categories, "allCategories should not be empty")

				// Check first category has aliased field
				category := categories[0].(map[string]interface{})
				require.Contains(t, category, "categoryType")
				require.NotContains(t, category, "kind", "original field name should not be present")
			},
		},
		{
			name:  "Nested object aliases",
			query: `query { nestedData: nestedType { identifier: id title: name childB: b { identifier: id title: name } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nestedData, ok := data["nestedData"].([]interface{})
				require.True(t, ok, "nestedData should be an array")
				require.NotEmpty(t, nestedData, "nestedData should not be empty")

				nestedItem := nestedData[0].(map[string]interface{})
				require.Contains(t, nestedItem, "identifier")
				require.Contains(t, nestedItem, "title")
				require.Contains(t, nestedItem, "childB")

				// Check nested object aliases
				childB := nestedItem["childB"].(map[string]interface{})
				require.Contains(t, childB, "identifier")
				require.Contains(t, childB, "title")

				// Ensure original field names are not present
				require.NotContains(t, nestedItem, "id")
				require.NotContains(t, nestedItem, "name")
				require.NotContains(t, nestedItem, "b")
			},
		},
		{
			name:  "Interface aliases",
			query: `query { pet: randomPet { identifier: id petName: name animalKind: kind ... on Cat { volumeLevel: meowVolume } ... on Dog { volumeLevel: barkVolume } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				pet, ok := data["pet"].(map[string]interface{})
				require.True(t, ok, "pet should be an object")
				require.NotEmpty(t, pet, "pet should not be empty")

				require.Contains(t, pet, "identifier")
				require.Contains(t, pet, "petName")
				require.Contains(t, pet, "animalKind")

				// Check if it has the volume level (either cat or dog)
				if _, hasCat := pet["volumeLevel"]; hasCat {
					require.Contains(t, pet, "volumeLevel")
					require.IsType(t, float64(0), pet["volumeLevel"]) // JSON numbers are float64
				}

				// Ensure original field names are not present
				require.NotContains(t, pet, "id")
				require.NotContains(t, pet, "name")
				require.NotContains(t, pet, "kind")
			},
		},
		{
			name:  "Union type aliases",
			query: `query { searchResults: randomSearchResult { ... on Product { productId: id productName: name cost: price } ... on User { userId: id userName: name } ... on Category { categoryId: id categoryName: name categoryType: kind } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				searchResults, ok := data["searchResults"].(map[string]interface{})
				require.True(t, ok, "searchResults should be an object")
				require.NotEmpty(t, searchResults, "searchResults should not be empty")

				// Check based on which union member was returned
				if productId, hasProduct := searchResults["productId"]; hasProduct {
					// Product case
					require.Contains(t, searchResults, "productName")
					require.Contains(t, searchResults, "cost")
					require.Equal(t, "product-random-1", productId)
					require.Equal(t, "Random Product", searchResults["productName"])
					require.Equal(t, 29.99, searchResults["cost"])
				} else if userId, hasUser := searchResults["userId"]; hasUser {
					// User case
					require.Contains(t, searchResults, "userName")
					require.Equal(t, "user-random-1", userId)
					require.Equal(t, "Random User", searchResults["userName"])
				} else if categoryId, hasCategory := searchResults["categoryId"]; hasCategory {
					// Category case
					require.Contains(t, searchResults, "categoryName")
					require.Contains(t, searchResults, "categoryType")
					require.Equal(t, "category-random-1", categoryId)
					require.Equal(t, "Random Category", searchResults["categoryName"])
					require.Equal(t, "ELECTRONICS", searchResults["categoryType"])
				} else {
					t.Fatal("searchResults should contain at least one union member with aliased fields")
				}

				// Ensure original field names are not present
				require.NotContains(t, searchResults, "id")
				require.NotContains(t, searchResults, "name")
				require.NotContains(t, searchResults, "price")
				require.NotContains(t, searchResults, "kind")
			},
		},
		{
			name:  "Mutation aliases",
			query: `mutation { newUser: createUser(input: $input) { userId: id fullName: name } }`,
			vars:  `{"variables": {"input": {"name": "John Doe"}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				newUser, ok := data["newUser"].(map[string]interface{})
				require.True(t, ok, "newUser should be an object")
				require.NotEmpty(t, newUser, "newUser should not be empty")

				require.Contains(t, newUser, "userId")
				require.Contains(t, newUser, "fullName")
				require.NotEmpty(t, newUser["userId"])
				require.Equal(t, "John Doe", newUser["fullName"])

				// Ensure original field names are not present
				require.NotContains(t, newUser, "id")
				require.NotContains(t, newUser, "name")
			},
		},
		{
			name:  "Enum field aliases",
			query: `query { bookCategories: categoriesByKind(kind: $kind) { identifier: id title: name type: kind } }`,
			vars:  `{"variables": {"kind": "BOOK"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bookCategories, ok := data["bookCategories"].([]interface{})
				require.True(t, ok, "bookCategories should be an array")
				require.NotEmpty(t, bookCategories, "bookCategories should not be empty")

				category := bookCategories[0].(map[string]interface{})
				require.Contains(t, category, "identifier")
				require.Contains(t, category, "title")
				require.Contains(t, category, "type")
				require.Equal(t, "BOOK", category["type"])

				// Ensure original field names are not present
				require.NotContains(t, category, "id")
				require.NotContains(t, category, "name")
				require.NotContains(t, category, "kind")
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
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Mapping:      testMapping(),
				Compiler:     compiler,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp struct {
				Data   map[string]interface{} `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")
			require.Empty(t, resp.Errors, "Response should not contain errors")
			require.NotEmpty(t, resp.Data, "Response should contain data")

			// Run the validation function
			tc.validate(t, resp.Data)
		})
	}
}

func Test_DataSource_Load_WithNullableFieldsType(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	t.Cleanup(cleanup)

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name:  "Query nullable fields type with all fields",
			query: `query { nullableFieldsType { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsType should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsType should not be empty")

				// Check required fields are present
				require.Contains(t, nullableFieldsType, "id")
				require.Contains(t, nullableFieldsType, "name")
				require.Contains(t, nullableFieldsType, "requiredString")
				require.Contains(t, nullableFieldsType, "requiredInt")

				require.NotEmpty(t, nullableFieldsType["id"], "id should not be empty")
				require.NotEmpty(t, nullableFieldsType["name"], "name should not be empty")
				require.NotEmpty(t, nullableFieldsType["requiredString"], "requiredString should not be empty")
				require.NotEmpty(t, nullableFieldsType["requiredInt"], "requiredInt should not be empty")

				// Check optional fields are present (but may be null)
				require.Contains(t, nullableFieldsType, "optionalString")
				require.Contains(t, nullableFieldsType, "optionalInt")
				require.Contains(t, nullableFieldsType, "optionalFloat")
				require.Contains(t, nullableFieldsType, "optionalBoolean")

				// Verify types of non-null optional fields
				if nullableFieldsType["optionalString"] != nil {
					require.IsType(t, "", nullableFieldsType["optionalString"])
				}
				if nullableFieldsType["optionalInt"] != nil {
					require.IsType(t, float64(0), nullableFieldsType["optionalInt"]) // JSON numbers are float64
				}
				if nullableFieldsType["optionalFloat"] != nil {
					require.IsType(t, float64(0), nullableFieldsType["optionalFloat"])
				}
				if nullableFieldsType["optionalBoolean"] != nil {
					require.IsType(t, false, nullableFieldsType["optionalBoolean"])
				}
			},
		},
		{
			name:  "Query nullable fields type by ID",
			query: `query($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString requiredString } }`,
			vars:  `{"variables":{"id":"full-data"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsTypeById"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsTypeById should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsTypeById should not be empty")

				require.Equal(t, "full-data", nullableFieldsType["id"])
				require.Equal(t, "Full Data by ID", nullableFieldsType["name"])
				require.Equal(t, "All fields populated", nullableFieldsType["optionalString"])
				require.Equal(t, "Required by ID", nullableFieldsType["requiredString"])
			},
		},
		{
			name:  "Query nullable fields type by ID with partial data",
			query: `query($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"id":"partial-data"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsTypeById"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsTypeById should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsTypeById should not be empty")

				require.Equal(t, "partial-data", nullableFieldsType["id"])
				require.Equal(t, "Partial Data by ID", nullableFieldsType["name"])
				require.Nil(t, nullableFieldsType["optionalString"], "optionalString should be null")
				require.NotNil(t, nullableFieldsType["optionalInt"], "optionalInt should not be null")
				require.Nil(t, nullableFieldsType["optionalFloat"], "optionalFloat should be null")
				require.NotNil(t, nullableFieldsType["optionalBoolean"], "optionalBoolean should not be null")
				require.Equal(t, "Partial required by ID", nullableFieldsType["requiredString"])
				require.Equal(t, float64(321), nullableFieldsType["requiredInt"]) // JSON numbers are float64
			},
		},
		{
			name:  "Query nullable fields type by ID with minimal data",
			query: `query($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"id":"minimal-data"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsTypeById"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsTypeById should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsTypeById should not be empty")

				require.Equal(t, "minimal-data", nullableFieldsType["id"])
				require.Equal(t, "Minimal Data by ID", nullableFieldsType["name"])
				require.Nil(t, nullableFieldsType["optionalString"], "optionalString should be null")
				require.Nil(t, nullableFieldsType["optionalInt"], "optionalInt should be null")
				require.Nil(t, nullableFieldsType["optionalFloat"], "optionalFloat should be null")
				require.Nil(t, nullableFieldsType["optionalBoolean"], "optionalBoolean should be null")
				require.Equal(t, "Only required fields", nullableFieldsType["requiredString"])
				require.Equal(t, float64(111), nullableFieldsType["requiredInt"]) // JSON numbers are float64
			},
		},
		{
			name:  "Query nullable fields type by ID - not found",
			query: `query($id: ID!) { nullableFieldsTypeById(id: $id) { id name optionalString requiredString } }`,
			vars:  `{"variables":{"id":"not-found"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType := data["nullableFieldsTypeById"]
				require.Nil(t, nullableFieldsType, "nullableFieldsTypeById should be null for not-found ID")
			},
		},
		{
			name:  "Query all nullable fields types",
			query: `query { allNullableFieldsTypes { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				allNullableFieldsTypes, ok := data["allNullableFieldsTypes"].([]interface{})
				require.True(t, ok, "allNullableFieldsTypes should be an array")
				require.Len(t, allNullableFieldsTypes, 3, "should return 3 nullable field types")

				// Check first entry (full data)
				firstEntry := allNullableFieldsTypes[0].(map[string]interface{})
				require.Equal(t, "nullable-1", firstEntry["id"])
				require.Equal(t, "Full Data Entry", firstEntry["name"])
				require.Equal(t, "Optional String Value", firstEntry["optionalString"])
				require.Equal(t, float64(42), firstEntry["optionalInt"])
				require.Equal(t, float64(3.14), math.Round(firstEntry["optionalFloat"].(float64)*100)/100) // round to 2 decimal places
				require.Equal(t, true, firstEntry["optionalBoolean"])
				require.Equal(t, "Required String 1", firstEntry["requiredString"])
				require.Equal(t, float64(100), firstEntry["requiredInt"])

				// Check second entry (partial data)
				secondEntry := allNullableFieldsTypes[1].(map[string]interface{})
				require.Equal(t, "nullable-2", secondEntry["id"])
				require.Equal(t, "Partial Data Entry", secondEntry["name"])
				require.Equal(t, "Only string is set", secondEntry["optionalString"])
				require.Nil(t, secondEntry["optionalInt"], "optionalInt should be null")
				require.Nil(t, secondEntry["optionalFloat"], "optionalFloat should be null")
				require.Equal(t, false, secondEntry["optionalBoolean"])
				require.Equal(t, "Required String 2", secondEntry["requiredString"])
				require.Equal(t, float64(200), secondEntry["requiredInt"])

				// Check third entry (minimal data)
				thirdEntry := allNullableFieldsTypes[2].(map[string]interface{})
				require.Equal(t, "nullable-3", thirdEntry["id"])
				require.Equal(t, "Minimal Data Entry", thirdEntry["name"])
				require.Nil(t, thirdEntry["optionalString"], "optionalString should be null")
				require.Nil(t, thirdEntry["optionalInt"], "optionalInt should be null")
				require.Nil(t, thirdEntry["optionalFloat"], "optionalFloat should be null")
				require.Nil(t, thirdEntry["optionalBoolean"], "optionalBoolean should be null")
				require.Equal(t, "Required String 3", thirdEntry["requiredString"])
				require.Equal(t, float64(300), thirdEntry["requiredInt"])
			},
		},
		{
			name:  "Query nullable fields type with filter",
			query: `query($filter: NullableFieldsFilter!) { nullableFieldsTypeWithFilter(filter: $filter) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"filter":{"name":"TestFilter","optionalString":"FilteredString","includeNulls":true}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsTypes, ok := data["nullableFieldsTypeWithFilter"].([]interface{})
				require.True(t, ok, "nullableFieldsTypeWithFilter should be an array")
				require.Len(t, nullableFieldsTypes, 3, "should return 3 filtered nullable field types")

				for i, item := range nullableFieldsTypes {
					entry := item.(map[string]interface{})
					require.Equal(t, fmt.Sprintf("filtered-%d", i+1), entry["id"])
					require.Equal(t, fmt.Sprintf("TestFilter - %d", i+1), entry["name"])
					require.Equal(t, "FilteredString", entry["optionalString"])
					require.Equal(t, fmt.Sprintf("Required filtered %d", i+1), entry["requiredString"])
					require.Equal(t, float64((i+1)*1000), entry["requiredInt"])
				}
			},
		},
		{
			name:  "Create nullable fields type mutation",
			query: `mutation($input: NullableFieldsInput!) { createNullableFieldsType(input: $input) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"input":{"name":"Created Type","optionalString":"Optional Value","optionalInt":42,"optionalFloat":3.14,"optionalBoolean":true,"requiredString":"Required Value","requiredInt":100}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				createdType, ok := data["createNullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "createNullableFieldsType should be an object")
				require.NotEmpty(t, createdType, "createNullableFieldsType should not be empty")

				require.Contains(t, createdType["id"], "nullable-") // ID should start with "nullable-"
				require.Equal(t, "Created Type", createdType["name"])
				require.Equal(t, "Optional Value", createdType["optionalString"])
				require.Equal(t, float64(42), createdType["optionalInt"])
				require.Equal(t, float64(3.14), math.Round(createdType["optionalFloat"].(float64)*100)/100) // round to 2 decimal places
				require.Equal(t, true, createdType["optionalBoolean"])
				require.Equal(t, "Required Value", createdType["requiredString"])
				require.Equal(t, float64(100), createdType["requiredInt"])
			},
		},
		{
			name:  "Create nullable fields type mutation with minimal input",
			query: `mutation($input: NullableFieldsInput!) { createNullableFieldsType(input: $input) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"input":{"name":"Minimal Type","requiredString":"Only Required","requiredInt":200}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				createdType, ok := data["createNullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "createNullableFieldsType should be an object")
				require.NotEmpty(t, createdType, "createNullableFieldsType should not be empty")

				require.Contains(t, createdType["id"], "nullable-") // ID should start with "nullable-"
				require.Equal(t, "Minimal Type", createdType["name"])
				require.Nil(t, createdType["optionalString"], "optionalString should be null")
				require.Nil(t, createdType["optionalInt"], "optionalInt should be null")
				require.Nil(t, createdType["optionalFloat"], "optionalFloat should be null")
				require.Nil(t, createdType["optionalBoolean"], "optionalBoolean should be null")
				require.Equal(t, "Only Required", createdType["requiredString"])
				require.Equal(t, float64(200), createdType["requiredInt"])
			},
		},
		{
			name:  "Update nullable fields type mutation",
			query: `mutation($id: ID!, $input: NullableFieldsInput!) { updateNullableFieldsType(id: $id, input: $input) { id name optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
			vars:  `{"variables":{"id":"test-update","input":{"name":"Updated Type","optionalString":"Updated Optional","optionalInt":999,"requiredString":"Updated Required","requiredInt":500}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				updatedType, ok := data["updateNullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "updateNullableFieldsType should be an object")
				require.NotEmpty(t, updatedType, "updateNullableFieldsType should not be empty")

				require.Equal(t, "test-update", updatedType["id"])
				require.Equal(t, "Updated Type", updatedType["name"])
				require.Equal(t, "Updated Optional", updatedType["optionalString"])
				require.Equal(t, float64(999), updatedType["optionalInt"])
				require.Nil(t, updatedType["optionalFloat"], "optionalFloat should be null")
				require.Nil(t, updatedType["optionalBoolean"], "optionalBoolean should be null")
				require.Equal(t, "Updated Required", updatedType["requiredString"])
				require.Equal(t, float64(500), updatedType["requiredInt"])
			},
		},
		{
			name:  "Update nullable fields type mutation - non-existent ID",
			query: `mutation($id: ID!, $input: NullableFieldsInput!) { updateNullableFieldsType(id: $id, input: $input) { id name optionalString requiredString } }`,
			vars:  `{"variables":{"id":"non-existent","input":{"name":"Should Not Exist","requiredString":"Not Created","requiredInt":0}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				updatedType := data["updateNullableFieldsType"]
				require.Nil(t, updatedType, "updateNullableFieldsType should be null for non-existent ID")
			},
		},
		{
			name:  "Query nullable fields with only optional fields",
			query: `query { nullableFieldsType { optionalString optionalInt optionalFloat optionalBoolean } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsType should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsType should not be empty")

				// Should only contain the requested optional fields
				require.Contains(t, nullableFieldsType, "optionalString")
				require.Contains(t, nullableFieldsType, "optionalInt")
				require.Contains(t, nullableFieldsType, "optionalFloat")
				require.Contains(t, nullableFieldsType, "optionalBoolean")

				// Should not contain other fields
				require.NotContains(t, nullableFieldsType, "id")
				require.NotContains(t, nullableFieldsType, "name")
				require.NotContains(t, nullableFieldsType, "requiredString")
				require.NotContains(t, nullableFieldsType, "requiredInt")
			},
		},
		{
			name:  "Query nullable fields with partial selection",
			query: `query { nullableFieldsType { id name optionalString requiredString } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsType should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsType should not be empty")

				// Should contain the requested fields
				require.Contains(t, nullableFieldsType, "id")
				require.Contains(t, nullableFieldsType, "name")
				require.Contains(t, nullableFieldsType, "optionalString")
				require.Contains(t, nullableFieldsType, "requiredString")

				// Should not contain other fields
				require.NotContains(t, nullableFieldsType, "optionalInt")
				require.NotContains(t, nullableFieldsType, "optionalFloat")
				require.NotContains(t, nullableFieldsType, "optionalBoolean")
				require.NotContains(t, nullableFieldsType, "requiredInt")
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
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Mapping:      testMapping(),
				Compiler:     compiler,
			})
			require.NoError(t, err)

			// Execute the query through our datasource
			output := new(bytes.Buffer)
			input := fmt.Sprintf(`{"query":%q,"body":%s}`, tc.query, tc.vars)
			err = ds.Load(context.Background(), []byte(input), output)
			require.NoError(t, err)

			// Parse the response
			var resp struct {
				Data   map[string]interface{} `json:"data"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors,omitempty"`
			}

			err = json.Unmarshal(output.Bytes(), &resp)
			require.NoError(t, err, "Failed to unmarshal response")
			require.Empty(t, resp.Errors, "Response should not contain errors")
			require.NotEmpty(t, resp.Data, "Response should contain data")

			// Run the validation function
			tc.validate(t, resp.Data)
		})
	}
}
