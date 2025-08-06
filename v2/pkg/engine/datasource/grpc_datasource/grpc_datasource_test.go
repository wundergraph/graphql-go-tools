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
			Service: "Products",
			QueryRPCs: RPCConfigMap{
				"complexFilterType": {
					RPC:      "QueryComplexFilterType",
					Request:  "QueryComplexFilterTypeRequest",
					Response: "QueryComplexFilterTypeResponse",
				},
			},
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
			Service: "Products",
			QueryRPCs: RPCConfigMap{
				"complexFilterType": {
					RPC:      "QueryComplexFilterType",
					Request:  "QueryComplexFilterTypeRequest",
					Response: "QueryComplexFilterTypeResponse",
				},
			},
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
						ArgumentMappings: map[string]string{
							"filter": "filter",
						},
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
			Service: "Products",
			QueryRPCs: RPCConfigMap{
				"complexFilterType": {
					RPC:      "QueryComplexFilterType",
					Request:  "QueryComplexFilterTypeRequest",
					Response: "QueryComplexFilterTypeResponse",
				},
			},
			Fields: map[string]FieldMap{
				"Query": {
					"complexFilterType": {
						TargetName: "complex_filter_type",
						ArgumentMappings: map[string]string{
							"filter": "filter",
						},
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
		Mapping: &GRPCMapping{
			Service: "Products",
			QueryRPCs: RPCConfigMap{
				"user": {
					RPC:      "QueryUser",
					Request:  "QueryUserRequest",
					Response: "QueryUserResponse",
				},
			},
			Fields: map[string]FieldMap{
				"Query": {
					"user": {
						TargetName: "user",
					},
				},
				"User": {
					"id": {
						TargetName: "id",
					},
					"name": {
						TargetName: "name",
					},
				},
			},
		},
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

	productMsg, ok := compiler.doc.MessageByName("Product")
	require.True(t, ok)
	productMessageDesc := productMsg.Desc
	productMessage := dynamicpb.NewMessage(productMessageDesc)
	productMessage.Set(productMessageDesc.Fields().ByName("id"), protoref.ValueOfString("123"))
	productMessage.Set(productMessageDesc.Fields().ByName("name"), protoref.ValueOfString("test"))
	productMessage.Set(productMessageDesc.Fields().ByName("price"), protoref.ValueOfFloat64(123.45))

	responseMsg, ok := compiler.doc.MessageByName("LookupProductByIdResponse")
	require.True(t, ok)
	responseMessageDesc := responseMsg.Desc
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
			query: `query { specificUser: user(id: $id) { userId1: id userId2: id userName1: name userName2: name } }`,
			vars:  `{"variables": {"id": "123"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				user, ok := data["specificUser"].(map[string]interface{})
				require.True(t, ok, "specificUser should be an object")
				require.NotEmpty(t, user, "specificUser should not be empty")

				require.Contains(t, user, "userId1")
				require.Contains(t, user, "userId2")
				require.Contains(t, user, "userName1")
				require.Contains(t, user, "userName2")

				// Check that aliases have the same values
				require.Equal(t, user["userId1"], user["userId2"])
				require.Equal(t, user["userName1"], user["userName2"])
				require.Equal(t, "123", user["userId1"])
				require.Equal(t, "User 123", user["userName1"])

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
		{
			name:  "Multiple aliases for the same field",
			query: `query { users { id name1: name name2: name name3: name } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				users, ok := data["users"].([]interface{})
				require.True(t, ok, "users should be an array")
				require.NotEmpty(t, users, "users should not be empty")

				user := users[0].(map[string]interface{})
				require.Contains(t, user, "id")
				require.Contains(t, user, "name1")
				require.Contains(t, user, "name2")
				require.Contains(t, user, "name3")

				// All aliases should have the same value
				require.Equal(t, user["name1"], user["name2"])
				require.Equal(t, user["name2"], user["name3"])
				require.NotEmpty(t, user["name1"])

				// Original field name should not be present
				require.NotContains(t, user, "name")
			},
		},
		{
			name:  "Multiple aliases for the same field with arguments",
			query: `query($id1: ID!, $id2: ID!, $id3: ID!) { user1: user(id: $id1) { id name } user2: user(id: $id2) { id name } sameUser: user(id: $id3) { userId: id userName: name } }`,
			vars:  `{"variables": {"id1": "123", "id2": "456", "id3": "123"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				user1, ok := data["user1"].(map[string]interface{})
				require.True(t, ok, "user1 should be an object")
				require.NotEmpty(t, user1, "user1 should not be empty")

				user2, ok := data["user2"].(map[string]interface{})
				require.True(t, ok, "user2 should be an object")
				require.NotEmpty(t, user2, "user2 should not be empty")

				sameUser, ok := data["sameUser"].(map[string]interface{})
				require.True(t, ok, "sameUser should be an object")
				require.NotEmpty(t, sameUser, "sameUser should not be empty")

				// user1 and sameUser should have the same ID since they query the same user
				require.Equal(t, user1["id"], sameUser["userId"])
				require.Equal(t, user1["name"], sameUser["userName"])

				// user2 should have different ID
				require.NotEqual(t, user1["id"], user2["id"])

				// Verify expected values
				require.Equal(t, "123", user1["id"])
				require.Equal(t, "User 123", user1["name"])
				require.Equal(t, "456", user2["id"])
				require.Equal(t, "User 456", user2["name"])
				require.Equal(t, "123", sameUser["userId"])
				require.Equal(t, "User 123", sameUser["userName"])
			},
		},
		{
			name:  "Multiple aliases for the same field in nested objects",
			query: `query { nestedType { id name1: name name2: name b { id title1: name title2: name c { id label1: name label2: name } } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nestedType, ok := data["nestedType"].([]interface{})
				require.True(t, ok, "nestedType should be an array")
				require.NotEmpty(t, nestedType, "nestedType should not be empty")

				nestedItem := nestedType[0].(map[string]interface{})
				require.Contains(t, nestedItem, "id")
				require.Contains(t, nestedItem, "name1")
				require.Contains(t, nestedItem, "name2")

				// Check that aliases have the same value
				require.Equal(t, nestedItem["name1"], nestedItem["name2"])
				require.NotEmpty(t, nestedItem["name1"])

				// Check nested object B
				childB := nestedItem["b"].(map[string]interface{})
				require.Contains(t, childB, "id")
				require.Contains(t, childB, "title1")
				require.Contains(t, childB, "title2")
				require.Equal(t, childB["title1"], childB["title2"])

				// Check nested object C
				childC := childB["c"].(map[string]interface{})
				require.Contains(t, childC, "id")
				require.Contains(t, childC, "label1")
				require.Contains(t, childC, "label2")
				require.Equal(t, childC["label1"], childC["label2"])

				// Ensure original field names are not present
				require.NotContains(t, nestedItem, "name")
				require.NotContains(t, childB, "name")
				require.NotContains(t, childC, "name")
			},
		},
		{
			name:  "Multiple aliases for the same field in interface fragments",
			query: `query { randomPet { id name1: name name2: name kind ... on Cat { volume1: meowVolume volume2: meowVolume } ... on Dog { volume1: barkVolume volume2: barkVolume } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				pet, ok := data["randomPet"].(map[string]interface{})
				require.True(t, ok, "randomPet should be an object")
				require.NotEmpty(t, pet, "randomPet should not be empty")

				require.Contains(t, pet, "id")
				require.Contains(t, pet, "name1")
				require.Contains(t, pet, "name2")
				require.Contains(t, pet, "kind")

				// Check that aliases have the same value
				require.Equal(t, pet["name1"], pet["name2"])
				require.NotEmpty(t, pet["name1"])

				// Check type-specific aliases based on what's present
				if volume1, hasVolume1 := pet["volume1"]; hasVolume1 {
					require.IsType(t, float64(0), volume1, "volume1 should be a number")
					require.Contains(t, pet, "volume2")
					require.Equal(t, volume1, pet["volume2"])
				}

				// Ensure original field names are not present
				require.NotContains(t, pet, "name")
				require.NotContains(t, pet, "meowVolume")
				require.NotContains(t, pet, "barkVolume")
			},
		},
		{
			name:  "Multiple aliases for the same field call with identical arguments",
			query: `query($id: ID!) { user1: user(id: $id) { id name1: name name2: name name3: name } user2: user(id: $id) { userId1: id userId2: id userName1: name userName2: name } }`,
			vars:  `{"variables": {"id": "123"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				user1, ok := data["user1"].(map[string]interface{})
				require.True(t, ok, "user1 should be an object")
				require.NotEmpty(t, user1, "user1 should not be empty")

				user2, ok := data["user2"].(map[string]interface{})
				require.True(t, ok, "user2 should be an object")
				require.NotEmpty(t, user2, "user2 should not be empty")

				// Both users should have the same data since they query the same ID
				require.Equal(t, user1["id"], user2["userId1"])
				require.Equal(t, user1["id"], user2["userId2"])
				require.Equal(t, user1["name1"], user2["userName1"])
				require.Equal(t, user1["name1"], user2["userName2"])

				// Check that aliases in user1 have the same value
				require.Equal(t, user1["name1"], user1["name2"])
				require.Equal(t, user1["name2"], user1["name3"])
				require.NotEmpty(t, user1["name1"])

				// Check that aliases in user2 have the same value
				require.Equal(t, user2["userId1"], user2["userId2"])
				require.Equal(t, user2["userName1"], user2["userName2"])
				require.NotEmpty(t, user2["userId1"])

				// Verify expected values
				require.Equal(t, "123", user1["id"])
				require.Equal(t, "User 123", user1["name1"])
				require.Equal(t, "123", user2["userId1"])
				require.Equal(t, "User 123", user2["userName1"])

				// Ensure original field names are not present in user2 (since it only uses aliases)
				require.NotContains(t, user2, "id")
				require.NotContains(t, user2, "name")
			},
		},
		{
			name:  "Multiple aliases for the same field in union fragments",
			query: `query { randomSearchResult { ... on Product { id name1: name name2: name price1: price } ... on User { id name1: name name2: name } ... on Category { id name1: name name2: name kind1: kind kind2: kind } } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				searchResult, ok := data["randomSearchResult"].(map[string]interface{})
				require.True(t, ok, "randomSearchResult should be an object")
				require.NotEmpty(t, searchResult, "randomSearchResult should not be empty")

				require.Contains(t, searchResult, "id")

				// Check that name aliases have the same value (only if they exist)
				if name1, hasName1 := searchResult["name1"]; hasName1 {
					require.NotEmpty(t, name1, "name1 should not be empty")
					require.Contains(t, searchResult, "name2")
					require.Equal(t, name1, searchResult["name2"])
				}

				// Check type-specific aliases
				if price1, hasPrice1 := searchResult["price1"]; hasPrice1 {
					// This is a Product
					require.IsType(t, float64(0), price1, "price1 should be a number")
				}

				if kind1, hasKind1 := searchResult["kind1"]; hasKind1 {
					// This is a Category
					require.IsType(t, "", kind1, "kind1 should be a string")
					require.Contains(t, searchResult, "kind2")
					require.Equal(t, kind1, searchResult["kind2"])
				}

				// Ensure original field names are not present
				require.NotContains(t, searchResult, "name")
				require.NotContains(t, searchResult, "price")
				require.NotContains(t, searchResult, "kind")
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
			name:  "Query nullable fields type with all fields and aliases",
			query: `query { nullableFieldsType { id name optionalString1: optionalString optionalInt1: optionalInt optionalFloat1: optionalFloat optionalBoolean1: optionalBoolean requiredString1: requiredString requiredInt1: requiredInt } }`,
			vars:  "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				nullableFieldsType, ok := data["nullableFieldsType"].(map[string]interface{})
				require.True(t, ok, "nullableFieldsType should be an object")
				require.NotEmpty(t, nullableFieldsType, "nullableFieldsType should not be empty")

				// Check required fields are present
				require.Contains(t, nullableFieldsType, "id")
				require.Contains(t, nullableFieldsType, "name")
				require.Contains(t, nullableFieldsType, "requiredString1")
				require.Contains(t, nullableFieldsType, "requiredInt1")

				require.NotEmpty(t, nullableFieldsType["id"], "id should not be empty")
				require.NotEmpty(t, nullableFieldsType["name"], "name should not be empty")
				require.NotEmpty(t, nullableFieldsType["requiredString1"], "requiredString1 should not be empty")
				require.NotEmpty(t, nullableFieldsType["requiredInt1"], "requiredInt1 should not be empty")

				// Check optional fields are present (but may be null)
				require.Contains(t, nullableFieldsType, "optionalString1")
				require.Contains(t, nullableFieldsType, "optionalInt1")
				require.Contains(t, nullableFieldsType, "optionalFloat1")
				require.Contains(t, nullableFieldsType, "optionalBoolean1")

				// Verify types of non-null optional fields
				if nullableFieldsType["optionalString1"] != nil {
					require.IsType(t, "", nullableFieldsType["optionalString1"])
				}
				if nullableFieldsType["optionalInt1"] != nil {
					require.IsType(t, float64(0), nullableFieldsType["optionalInt1"]) // JSON numbers are float64
				}
				if nullableFieldsType["optionalFloat1"] != nil {
					require.IsType(t, float64(0), nullableFieldsType["optionalFloat1"])
				}
				if nullableFieldsType["optionalBoolean1"] != nil {
					require.IsType(t, false, nullableFieldsType["optionalBoolean1"])
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
				require.InDelta(t, math.MaxFloat64, firstEntry["optionalFloat"], 0.01)
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
				require.InDelta(t, float64(3.14), createdType["optionalFloat"], 0.01)
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

func Test_DataSource_Load_WithNestedLists(t *testing.T) {
	conn, cleanup := setupTestGRPCServer(t)
	defer cleanup()

	testCases := []struct {
		name     string
		query    string
		vars     string
		validate func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "Should handle BlogPost with single lists of different nullability",
			query: `query {
				blogPost {
					id
					title
					content
					tags
					optionalTags
					categories
					keywords
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				blogPost, ok := data["blogPost"].(map[string]interface{})
				require.True(t, ok, "blogPost should be an object")

				// Check required fields
				require.NotEmpty(t, blogPost["id"])
				require.NotEmpty(t, blogPost["title"])
				require.NotEmpty(t, blogPost["content"])

				// Check required list with required items
				tags, ok := blogPost["tags"].([]interface{})
				require.True(t, ok, "tags should be an array")
				require.NotEmpty(t, tags, "tags should not be empty")

				// Check optional list with required items (can be null or array)
				if optionalTags := blogPost["optionalTags"]; optionalTags != nil {
					optionalTagsArr, ok := optionalTags.([]interface{})
					require.True(t, ok, "optionalTags should be an array if present")
					require.NotEmpty(t, optionalTagsArr, "optionalTags should not be empty if present")
				}

				// Check required list with optional items
				_, ok = blogPost["categories"].([]interface{})
				require.True(t, ok, "categories should be an array")
				// categories can contain null items

				// Check optional list with optional items (can be null or array)
				if keywords := blogPost["keywords"]; keywords != nil {
					_, ok := keywords.([]interface{})
					require.True(t, ok, "keywords should be an array if present")
					// keywords array can contain null items
				}
			},
		},
		{
			name: "Should handle BlogPost with scalar type lists",
			query: `query {
				blogPost {
					id
					title
					viewCounts
					ratings
					isPublished
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				blogPost, ok := data["blogPost"].(map[string]interface{})
				require.True(t, ok, "blogPost should be an object")

				// Check required list of required ints
				viewCounts, ok := blogPost["viewCounts"].([]interface{})
				require.True(t, ok, "viewCounts should be an array")
				require.NotEmpty(t, viewCounts, "viewCounts should not be empty")
				for _, count := range viewCounts {
					require.IsType(t, float64(0), count, "viewCounts items should be numbers")
				}

				// Check optional list of optional floats
				if ratings := blogPost["ratings"]; ratings != nil {
					_, ok := ratings.([]interface{})
					require.True(t, ok, "ratings should be an array if present")
					// ratings can contain null values
				}

				// Check optional list of required booleans
				if isPublished := blogPost["isPublished"]; isPublished != nil {
					isPublishedArr, ok := isPublished.([]interface{})
					require.True(t, ok, "isPublished should be an array if present")
					for _, published := range isPublishedArr {
						require.IsType(t, true, published, "isPublished items should be booleans")
					}
				}
			},
		},
		{
			name: "Should handle BlogPost with nested lists",
			query: `query {
				blogPost {
					id
					title
					tagGroups
					relatedTopics
					commentThreads
					suggestions
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				blogPost, ok := data["blogPost"].(map[string]interface{})
				require.True(t, ok, "blogPost should be an object")

				// Check required list of required lists with required items
				tagGroups, ok := blogPost["tagGroups"].([]interface{})
				require.True(t, ok, "tagGroups should be an array")
				require.NotEmpty(t, tagGroups, "tagGroups should not be empty")
				for _, group := range tagGroups {
					groupArr, ok := group.([]interface{})
					require.True(t, ok, "tagGroups items should be arrays")
					require.NotEmpty(t, groupArr, "tagGroups inner arrays should not be empty")
					for _, tag := range groupArr {
						require.IsType(t, "", tag, "tags should be strings")
					}
				}

				// Check required list of optional lists with required items
				_, ok = blogPost["relatedTopics"].([]interface{})
				require.True(t, ok, "relatedTopics should be an array")
				// relatedTopics can contain null inner arrays

				// Check required list of required lists with optional items
				commentThreads, ok := blogPost["commentThreads"].([]interface{})
				require.True(t, ok, "commentThreads should be an array")
				require.NotEmpty(t, commentThreads, "commentThreads should not be empty")
				for _, thread := range commentThreads {
					_, ok := thread.([]interface{})
					require.True(t, ok, "commentThreads items should be arrays")
					for _, item := range thread.([]interface{}) {
						require.IsType(t, "", item, "commentThreads items should be strings")
					}
				}

				// Check optional list of optional lists with optional items
				if suggestions := blogPost["suggestions"]; suggestions != nil {
					_, ok := suggestions.([]interface{})
					require.True(t, ok, "suggestions should be an array if present")
					for _, suggestion := range suggestions.([]interface{}) {
						_, ok := suggestion.([]interface{})
						require.True(t, ok, "suggestions items should be arrays")
					}
				}
			},
		},
		{
			name: "Should handle Author with single lists",
			query: `query {
				author {
					id
					name
					email
					skills
					languages
					socialLinks
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				author, ok := data["author"].(map[string]interface{})
				require.True(t, ok, "author should be an object")

				// Check required fields
				require.NotEmpty(t, author["id"])
				require.NotEmpty(t, author["name"])

				// Check required list with required items
				skills, ok := author["skills"].([]interface{})
				require.True(t, ok, "skills should be an array")
				require.NotEmpty(t, skills, "skills should not be empty")

				// Check required list with optional items
				_, ok = author["languages"].([]interface{})
				require.True(t, ok, "languages should be an array")
				// languages can contain null items

				// Check optional list with optional items
				if socialLinks := author["socialLinks"]; socialLinks != nil {
					_, ok := socialLinks.([]interface{})
					require.True(t, ok, "socialLinks should be an array if present")
					// socialLinks can contain null items
				}
			},
		},
		{
			name: "Should handle Author with nested lists",
			query: `query {
				author {
					id
					name
					teamsByProject
					collaborations
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				author, ok := data["author"].(map[string]interface{})
				require.True(t, ok, "author should be an object")

				// Check required list of required lists with required items
				teamsByProject, ok := author["teamsByProject"].([]interface{})
				require.True(t, ok, "teamsByProject should be an array")
				require.NotEmpty(t, teamsByProject, "teamsByProject should not be empty")
				for _, project := range teamsByProject {
					projectArr, ok := project.([]interface{})
					require.True(t, ok, "teamsByProject items should be arrays")
					require.NotEmpty(t, projectArr, "teamsByProject inner arrays should not be empty")
					for _, member := range projectArr {
						require.IsType(t, "", member, "team members should be strings")
					}
				}

				// Check optional list of optional lists with optional items
				if collaborations := author["collaborations"]; collaborations != nil {
					_, ok := collaborations.([]interface{})
					require.True(t, ok, "collaborations should be an array if present")
					// collaborations can contain null inner arrays and null items
				}
			},
		},
		{
			name: "Should handle BlogPost query by ID",
			query: `query($id: ID!) {
				blogPostById(id: $id) {
					id
					title
					content
					tags
					tagGroups
				}
			}`,
			vars: `{"variables":{"id":"test-blog-1"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				blogPost, ok := data["blogPostById"].(map[string]interface{})
				require.True(t, ok, "blogPostById should be an object")
				require.Equal(t, "test-blog-1", blogPost["id"])
				require.NotEmpty(t, blogPost["title"])
				require.NotEmpty(t, blogPost["tags"])
				require.NotEmpty(t, blogPost["tagGroups"])
			},
		},
		{
			name: "Should handle Author query by ID",
			query: `query($id: ID!) {
				authorById(id: $id) {
					id
					name
					skills
					teamsByProject
				}
			}`,
			vars: `{"variables":{"id":"test-author-1"}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				author, ok := data["authorById"].(map[string]interface{})
				require.True(t, ok, "authorById should be an object")
				require.Equal(t, "test-author-1", author["id"])
				require.NotEmpty(t, author["name"])
				require.NotEmpty(t, author["skills"])
				require.NotEmpty(t, author["teamsByProject"])
			},
		},
		{
			name: "Should handle BlogPost filtered query",
			query: `query($filter: BlogPostFilter!) {
				blogPostsWithFilter(filter: $filter) {
					id
					title
					tags
					categories
					tagGroups
				}
			}`,
			vars: `{"variables":{"filter":{"title":"Test","hasCategories":true,"minTags":2}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				blogPosts, ok := data["blogPostsWithFilter"].([]interface{})
				require.True(t, ok, "blogPostsWithFilter should be an array")
				require.NotEmpty(t, blogPosts, "blogPostsWithFilter should not be empty")

				for _, post := range blogPosts {
					blogPost, ok := post.(map[string]interface{})
					require.True(t, ok, "each post should be an object")
					require.NotEmpty(t, blogPost["id"])
					require.NotEmpty(t, blogPost["title"])
					require.NotEmpty(t, blogPost["tags"])
					require.NotEmpty(t, blogPost["categories"])
					require.NotEmpty(t, blogPost["tagGroups"])
				}
			},
		},
		{
			name: "Should handle Author filtered query",
			query: `query($filter: AuthorFilter!) {
				authorsWithFilter(filter: $filter) {
					id
					name
					skills
					teamsByProject
				}
			}`,
			vars: `{"variables":{"filter":{"name":"Test","hasTeams":true,"skillCount":3}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				authors, ok := data["authorsWithFilter"].([]interface{})
				require.True(t, ok, "authorsWithFilter should be an array")
				require.NotEmpty(t, authors, "authorsWithFilter should not be empty")

				for _, auth := range authors {
					author, ok := auth.(map[string]interface{})
					require.True(t, ok, "each author should be an object")
					require.NotEmpty(t, author["id"])
					require.NotEmpty(t, author["name"])
					require.NotEmpty(t, author["skills"])
					require.NotEmpty(t, author["teamsByProject"])
				}
			},
		},
		{
			name: "Should handle BlogPost creation mutation",
			query: `mutation($input: BlogPostInput!) {
				createBlogPost(input: $input) {
					id
					title
					content
					tags
					optionalTags
					tagGroups
					relatedTopics
				}
			}`,
			vars: `{"variables":{"input":{"title":"New Blog Post","content":"Content here","tags":["tech","programming"],"optionalTags":["optional1","optional2"],"categories":["Technology","Programming"],"keywords":["keyword1","keyword2"],"viewCounts":[100,200,300],"ratings":[4.5,5.0,3.8],"isPublished":[true,false,true],"tagGroups":[["tech","go"],["programming","backend"]],"relatedTopics":[["topic1","topic2"],["topic3"]],"commentThreads":[["comment1","comment2"],["comment3","comment4"]],"suggestions":[["suggestion1"],["suggestion2","suggestion3"]]}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				createBlogPost, ok := data["createBlogPost"].(map[string]interface{})
				require.True(t, ok, "createBlogPost should be an object")
				require.NotEmpty(t, createBlogPost["id"])
				require.Equal(t, "New Blog Post", createBlogPost["title"])
				require.Equal(t, "Content here", createBlogPost["content"])

				// Verify lists
				tags, ok := createBlogPost["tags"].([]interface{})
				require.True(t, ok, "tags should be an array")
				require.Contains(t, tags, "tech")
				require.Contains(t, tags, "programming")

				optionalTags, ok := createBlogPost["optionalTags"].([]interface{})
				require.True(t, ok, "optionalTags should be an array")
				require.Contains(t, optionalTags, "optional1")
				require.Contains(t, optionalTags, "optional2")

				// Verify nested lists
				tagGroups, ok := createBlogPost["tagGroups"].([]interface{})
				require.True(t, ok, "tagGroups should be an array")
				require.Len(t, tagGroups, 2)

				relatedTopics, ok := createBlogPost["relatedTopics"].([]interface{})
				require.True(t, ok, "relatedTopics should be an array")
				require.Len(t, relatedTopics, 2)
			},
		},
		{
			name: "Should handle Author creation mutation",
			query: `mutation($input: AuthorInput!) {
				createAuthor(input: $input) {
					id
					name
					email
					skills
					languages
					socialLinks
					teamsByProject
					collaborations
				}
			}`,
			vars: `{"variables":{"input":{"name":"New Author","email":"author@example.com","skills":["Go","GraphQL","gRPC"],"languages":["English","Spanish"],"socialLinks":["twitter.com/author","github.com/author"],"teamsByProject":[["Alice","Bob"],["Charlie","David","Eve"]],"collaborations":[["Project1","Project2"],["Project3"]]}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				createAuthor, ok := data["createAuthor"].(map[string]interface{})
				require.True(t, ok, "createAuthor should be an object")
				require.NotEmpty(t, createAuthor["id"])
				require.Equal(t, "New Author", createAuthor["name"])
				require.Equal(t, "author@example.com", createAuthor["email"])

				// Verify single lists
				skills, ok := createAuthor["skills"].([]interface{})
				require.True(t, ok, "skills should be an array")
				require.Contains(t, skills, "Go")
				require.Contains(t, skills, "GraphQL")
				require.Contains(t, skills, "gRPC")

				languages, ok := createAuthor["languages"].([]interface{})
				require.True(t, ok, "languages should be an array")
				require.Contains(t, languages, "English")
				require.Contains(t, languages, "Spanish")

				socialLinks, ok := createAuthor["socialLinks"].([]interface{})
				require.True(t, ok, "socialLinks should be an array")
				require.Contains(t, socialLinks, "twitter.com/author")
				require.Contains(t, socialLinks, "github.com/author")

				// Verify nested lists
				teamsByProject, ok := createAuthor["teamsByProject"].([]interface{})
				require.True(t, ok, "teamsByProject should be an array")
				require.Len(t, teamsByProject, 2)

				collaborations, ok := createAuthor["collaborations"].([]interface{})
				require.True(t, ok, "collaborations should be an array")
				require.Len(t, collaborations, 2)
			},
		},
		{
			name: "Should handle all BlogPosts query",
			query: `query {
				allBlogPosts {
					id
					title
					tags
					tagGroups
					viewCounts
					ratings
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				allBlogPosts, ok := data["allBlogPosts"].([]interface{})
				require.True(t, ok, "allBlogPosts should be an array")
				require.NotEmpty(t, allBlogPosts, "allBlogPosts should not be empty")

				for _, post := range allBlogPosts {
					blogPost, ok := post.(map[string]interface{})
					require.True(t, ok, "each post should be an object")
					require.NotEmpty(t, blogPost["id"])
					require.NotEmpty(t, blogPost["title"])
					require.NotEmpty(t, blogPost["tags"])
				}
			},
		},
		{
			name: "Should handle all Authors query",
			query: `query {
				allAuthors {
					id
					name
					skills
					teamsByProject
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				allAuthors, ok := data["allAuthors"].([]interface{})
				require.True(t, ok, "allAuthors should be an array")
				require.NotEmpty(t, allAuthors, "allAuthors should not be empty")

				for _, auth := range allAuthors {
					author, ok := auth.(map[string]interface{})
					require.True(t, ok, "each author should be an object")
					require.NotEmpty(t, author["id"])
					require.NotEmpty(t, author["name"])
					require.NotEmpty(t, author["skills"])
				}
			},
		},
		{
			name: "Should handle BlogPost creation with complex input lists and nested complex input lists",
			query: `mutation($input: BlogPostInput!) {
				createBlogPost(input: $input) {
					id
					title
					content
					tags
					optionalTags
					categories
					keywords
					viewCounts
					ratings
					isPublished
					tagGroups
					relatedTopics
					commentThreads
					suggestions
					relatedCategories {
						id
						name
						kind
					}
					contributors {
						id
						name
					}
					categoryGroups {
						id
						name
						kind
					}
				}
			}`,
			vars: `{"variables":{"input":{"title":"Complex Lists Blog Post","content":"Testing complex input lists","tags":["graphql","grpc","lists"],"optionalTags":["optional1","optional2"],"categories":["Technology","Programming"],"keywords":["nested","complex","types"],"viewCounts":[150,250,350],"ratings":[4.2,4.8,3.9],"isPublished":[true,false,true],"tagGroups":[["graphql","schema"],["grpc","protobuf"],["lists","arrays"]],"relatedTopics":[["backend","api"],["frontend","ui"]],"commentThreads":[["Great post!","Thanks for sharing"],["Very helpful","Keep it up"]],"suggestions":[["Add examples"],["More details","Better formatting"]],"relatedCategories":[{"name":"Web Development","kind":"ELECTRONICS"},{"name":"API Design","kind":"OTHER"}],"contributors":[{"name":"Alice Developer"},{"name":"Bob Engineer"}],"categoryGroups":[[{"name":"Backend","kind":"ELECTRONICS"},{"name":"Database","kind":"OTHER"}],[{"name":"Frontend","kind":"ELECTRONICS"}]]}}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				createBlogPost, ok := data["createBlogPost"].(map[string]interface{})
				require.True(t, ok, "createBlogPost should be an object")

				// Check basic fields from input
				require.NotEmpty(t, createBlogPost["id"])
				require.Equal(t, "Complex Lists Blog Post", createBlogPost["title"])
				require.Equal(t, "Testing complex input lists", createBlogPost["content"])

				// Check scalar lists from input
				tags, ok := createBlogPost["tags"].([]interface{})
				require.True(t, ok, "tags should be an array")
				require.Contains(t, tags, "graphql")
				require.Contains(t, tags, "grpc")
				require.Contains(t, tags, "lists")

				optionalTags, ok := createBlogPost["optionalTags"].([]interface{})
				require.True(t, ok, "optionalTags should be an array")
				require.Contains(t, optionalTags, "optional1")
				require.Contains(t, optionalTags, "optional2")

				categories, ok := createBlogPost["categories"].([]interface{})
				require.True(t, ok, "categories should be an array")
				require.Contains(t, categories, "Technology")
				require.Contains(t, categories, "Programming")

				keywords, ok := createBlogPost["keywords"].([]interface{})
				require.True(t, ok, "keywords should be an array")
				require.Contains(t, keywords, "nested")
				require.Contains(t, keywords, "complex")
				require.Contains(t, keywords, "types")

				// Check nested scalar lists from input
				tagGroups, ok := createBlogPost["tagGroups"].([]interface{})
				require.True(t, ok, "tagGroups should be an array")
				require.Len(t, tagGroups, 3)

				firstTagGroup, ok := tagGroups[0].([]interface{})
				require.True(t, ok, "first tag group should be an array")
				require.Contains(t, firstTagGroup, "graphql")
				require.Contains(t, firstTagGroup, "schema")

				relatedTopics, ok := createBlogPost["relatedTopics"].([]interface{})
				require.True(t, ok, "relatedTopics should be an array")
				require.Len(t, relatedTopics, 2)

				commentThreads, ok := createBlogPost["commentThreads"].([]interface{})
				require.True(t, ok, "commentThreads should be an array")
				require.Len(t, commentThreads, 2)

				suggestions, ok := createBlogPost["suggestions"].([]interface{})
				require.True(t, ok, "suggestions should be an array")
				require.Len(t, suggestions, 2)

				// Check single complex lists from input - converted from input types to output types
				// relatedCategories: [CategoryInput] -> [Category]
				relatedCategories, ok := createBlogPost["relatedCategories"].([]interface{})
				require.True(t, ok, "relatedCategories should be an array")
				require.Len(t, relatedCategories, 2)
				for i, cat := range relatedCategories {
					category, ok := cat.(map[string]interface{})
					require.True(t, ok, "each category should be an object")
					require.NotEmpty(t, category["id"])
					require.NotEmpty(t, category["name"])
					require.NotEmpty(t, category["kind"])
					switch i {
					case 0:
						require.Equal(t, "Web Development", category["name"])
						require.Equal(t, "ELECTRONICS", category["kind"])
					case 1:
						require.Equal(t, "API Design", category["name"])
						require.Equal(t, "OTHER", category["kind"])
					}
				}

				// contributors: [UserInput] -> [User]
				contributors, ok := createBlogPost["contributors"].([]interface{})
				require.True(t, ok, "contributors should be an array")
				require.Len(t, contributors, 2)
				for i, cont := range contributors {
					contributor, ok := cont.(map[string]interface{})
					require.True(t, ok, "each contributor should be an object")
					require.NotEmpty(t, contributor["id"])
					require.NotEmpty(t, contributor["name"])
					switch i {
					case 0:
						require.Equal(t, "Alice Developer", contributor["name"])
					case 1:
						require.Equal(t, "Bob Engineer", contributor["name"])
					}
				}

				// Check nested complex lists from input - converted from input types to output types
				// categoryGroups: [[CategoryInput!]] -> [[Category!]]
				categoryGroups, ok := createBlogPost["categoryGroups"].([]interface{})
				require.True(t, ok, "categoryGroups should be an array")
				require.Len(t, categoryGroups, 2)

				// First group should have 2 categories
				firstCategoryGroup, ok := categoryGroups[0].([]interface{})
				require.True(t, ok, "first category group should be an array")
				require.Len(t, firstCategoryGroup, 2)
				for i, cat := range firstCategoryGroup {
					category, ok := cat.(map[string]interface{})
					require.True(t, ok, "each category should be an object")
					require.NotEmpty(t, category["id"])
					require.NotEmpty(t, category["name"])
					require.NotEmpty(t, category["kind"])
					switch i {
					case 0:
						require.Equal(t, "Backend", category["name"])
						require.Equal(t, "ELECTRONICS", category["kind"])
					case 1:
						require.Equal(t, "Database", category["name"])
						require.Equal(t, "OTHER", category["kind"])
					}
				}

				// Second group should have 1 category
				secondCategoryGroup, ok := categoryGroups[1].([]interface{})
				require.True(t, ok, "second category group should be an array")
				require.Len(t, secondCategoryGroup, 1)
				category, ok := secondCategoryGroup[0].(map[string]interface{})
				require.True(t, ok, "category should be an object")
				require.NotEmpty(t, category["id"])
				require.Equal(t, "Frontend", category["name"])
				require.Equal(t, "ELECTRONICS", category["kind"])
			},
		},
		{
			name: "Should handle Author with complex lists and nested complex lists",
			query: `query {
				author {
					id
					name
					email
					writtenPosts {
						id
						title
						content
					}
					favoriteCategories {
						id
						name
						kind
					}
					relatedAuthors {
						id
						name
					}
					productReviews {
						id
						name
						price
					}
					authorGroups {
						id
						name
					}
					categoryPreferences {
						id
						name
						kind
					}
					projectTeams {
						id
						name
					}
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				author, ok := data["author"].(map[string]interface{})
				require.True(t, ok, "author should be an object")

				// Check basic fields
				require.NotEmpty(t, author["id"])
				require.NotEmpty(t, author["name"])

				// Check single complex lists
				// writtenPosts: [BlogPost] - Optional list of blog posts
				if writtenPosts := author["writtenPosts"]; writtenPosts != nil {
					writtenPostsArr, ok := writtenPosts.([]interface{})
					require.True(t, ok, "writtenPosts should be an array if present")
					for _, post := range writtenPostsArr {
						if post != nil { // posts can be null
							blogPost, ok := post.(map[string]interface{})
							require.True(t, ok, "each blog post should be an object")
							require.NotEmpty(t, blogPost["id"])
							require.NotEmpty(t, blogPost["title"])
							require.NotEmpty(t, blogPost["content"])
						}
					}
				}

				// favoriteCategories: [Category!]! - Required list of required categories
				favoriteCategories, ok := author["favoriteCategories"].([]interface{})
				require.True(t, ok, "favoriteCategories should be an array")
				require.NotEmpty(t, favoriteCategories, "favoriteCategories should not be empty")
				for _, cat := range favoriteCategories {
					category, ok := cat.(map[string]interface{})
					require.True(t, ok, "each category should be an object")
					require.NotEmpty(t, category["id"])
					require.NotEmpty(t, category["name"])
					require.NotEmpty(t, category["kind"])
				}

				// relatedAuthors: [User] - Optional list of related authors/collaborators
				if relatedAuthors := author["relatedAuthors"]; relatedAuthors != nil {
					relatedAuthorsArr, ok := relatedAuthors.([]interface{})
					require.True(t, ok, "relatedAuthors should be an array if present")
					for _, auth := range relatedAuthorsArr {
						if auth != nil { // authors can be null
							authorObj, ok := auth.(map[string]interface{})
							require.True(t, ok, "each author should be an object")
							require.NotEmpty(t, authorObj["id"])
							require.NotEmpty(t, authorObj["name"])
						}
					}
				}

				// productReviews: [Product] - Optional list of products they've reviewed
				if productReviews := author["productReviews"]; productReviews != nil {
					productReviewsArr, ok := productReviews.([]interface{})
					require.True(t, ok, "productReviews should be an array if present")
					for _, prod := range productReviewsArr {
						if prod != nil { // products can be null
							product, ok := prod.(map[string]interface{})
							require.True(t, ok, "each product should be an object")
							require.NotEmpty(t, product["id"])
							require.NotEmpty(t, product["name"])
							require.NotEmpty(t, product["price"])
						}
					}
				}

				// Nested complex lists
				// authorGroups: [[User!]] - Optional groups of required authors
				if authorGroups := author["authorGroups"]; authorGroups != nil {
					authorGroupsArr, ok := authorGroups.([]interface{})
					require.True(t, ok, "authorGroups should be an array if present")
					for _, group := range authorGroupsArr {
						if group != nil { // groups can be null
							groupArr, ok := group.([]interface{})
							require.True(t, ok, "authorGroups items should be arrays")
							for _, auth := range groupArr {
								authorObj, ok := auth.(map[string]interface{})
								require.True(t, ok, "each author should be an object")
								require.NotEmpty(t, authorObj["id"])
								require.NotEmpty(t, authorObj["name"])
							}
						}
					}
				}

				// categoryPreferences: [[Category!]!]! - Required groups of required category preferences
				categoryPreferences, ok := author["categoryPreferences"].([]interface{})
				require.True(t, ok, "categoryPreferences should be an array")
				require.NotEmpty(t, categoryPreferences, "categoryPreferences should not be empty")
				for _, group := range categoryPreferences {
					groupArr, ok := group.([]interface{})
					require.True(t, ok, "categoryPreferences items should be arrays")
					require.NotEmpty(t, groupArr, "categoryPreferences inner arrays should not be empty")
					for _, cat := range groupArr {
						category, ok := cat.(map[string]interface{})
						require.True(t, ok, "each category should be an object")
						require.NotEmpty(t, category["id"])
						require.NotEmpty(t, category["name"])
						require.NotEmpty(t, category["kind"])
					}
				}

				// projectTeams: [[User]] - Optional groups of optional users for projects
				if projectTeams := author["projectTeams"]; projectTeams != nil {
					projectTeamsArr, ok := projectTeams.([]interface{})
					require.True(t, ok, "projectTeams should be an array if present")
					for _, team := range projectTeamsArr {
						if team != nil { // teams can be null
							teamArr, ok := team.([]interface{})
							require.True(t, ok, "projectTeams items should be arrays")
							for _, user := range teamArr {
								if user != nil { // users can be null
									userObj, ok := user.(map[string]interface{})
									require.True(t, ok, "each user should be an object")
									require.NotEmpty(t, userObj["id"])
									require.NotEmpty(t, userObj["name"])
								}
							}
						}
					}
				}
			},
		},
		{
			name: "Should handle bulk search authors with nullable list parameter",
			query: `query($filters: [AuthorFilter!]) {
				bulkSearchAuthors(filters: $filters) {
					id
					name
					skills
					languages
					teamsByProject
					favoriteCategories {
						id
						name
						kind
					}
					categoryPreferences {
						id
						name
						kind
					}
				}
			}`,
			vars: `{"variables":{"filters":[{"name":"TestAuthor","hasTeams":true,"skillCount":4},{"hasTeams":false,"skillCount":2}]}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkSearchAuthors, ok := data["bulkSearchAuthors"].([]interface{})
				require.True(t, ok, "bulkSearchAuthors should be an array")
				require.Len(t, bulkSearchAuthors, 4, "Should return 2 authors per filter = 4 total")

				for i, auth := range bulkSearchAuthors {
					author, ok := auth.(map[string]interface{})
					require.True(t, ok, "each author should be an object")
					require.NotEmpty(t, author["id"])
					require.NotEmpty(t, author["name"])

					// Check skills array
					skills, ok := author["skills"].([]interface{})
					require.True(t, ok, "skills should be an array")
					if i < 2 { // First filter has skillCount: 4
						require.Len(t, skills, 4, "First filter should generate 4 skills")
					} else { // Second filter has skillCount: 2
						require.Len(t, skills, 2, "Second filter should generate 2 skills")
					}

					// Check nested list teamsByProject
					teamsByProject, ok := author["teamsByProject"].([]interface{})
					require.True(t, ok, "teamsByProject should be an array")
					if i < 2 { // First filter has hasTeams: true
						require.NotEmpty(t, teamsByProject, "First filter should have teams")
					} else { // Second filter has hasTeams: false
						require.Empty(t, teamsByProject, "Second filter should have no teams")
					}

					// Check complex list favoriteCategories
					favoriteCategories, ok := author["favoriteCategories"].([]interface{})
					require.True(t, ok, "favoriteCategories should be an array")
					require.Len(t, favoriteCategories, 1, "Each author should have 1 favorite category")

					// Check nested complex list categoryPreferences
					categoryPreferences, ok := author["categoryPreferences"].([]interface{})
					require.True(t, ok, "categoryPreferences should be an array")
					require.Len(t, categoryPreferences, 1, "Each author should have 1 category preference group")
				}
			},
		},
		{
			name: "Should handle bulk search authors with null parameter",
			query: `query($filters: [AuthorFilter!]) {
				bulkSearchAuthors(filters: $filters) {
					id
					name
				}
			}`,
			vars: `{"variables":{"filters":null}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkSearchAuthors, ok := data["bulkSearchAuthors"].([]interface{})
				require.True(t, ok, "bulkSearchAuthors should be an array")
				require.Empty(t, bulkSearchAuthors, "Should return empty array when filters is null")
			},
		},
		{
			name: "Should handle bulk search blog posts with nullable list parameter",
			query: `query($filters: [BlogPostFilter!]) {
				bulkSearchBlogPosts(filters: $filters) {
					id
					title
					content
					tags
					categories
					viewCounts
					tagGroups
					relatedTopics
					commentThreads
					relatedCategories {
						id
						name
						kind
					}
					contributors {
						id
						name
					}
					categoryGroups {
						id
						name
						kind
					}
				}
			}`,
			vars: `{"variables":{"filters":[{"title":"TestPost","hasCategories":true,"minTags":3},{"hasCategories":false,"minTags":1}]}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkSearchBlogPosts, ok := data["bulkSearchBlogPosts"].([]interface{})
				require.True(t, ok, "bulkSearchBlogPosts should be an array")
				require.Len(t, bulkSearchBlogPosts, 4, "Should return 2 posts per filter = 4 total")

				for i, post := range bulkSearchBlogPosts {
					blogPost, ok := post.(map[string]interface{})
					require.True(t, ok, "each blog post should be an object")
					require.NotEmpty(t, blogPost["id"])
					require.NotEmpty(t, blogPost["title"])
					require.NotEmpty(t, blogPost["content"])

					// Check tags array based on minTags filter
					tags, ok := blogPost["tags"].([]interface{})
					require.True(t, ok, "tags should be an array")
					if i < 2 { // First filter has minTags: 3
						require.Len(t, tags, 3, "First filter should generate 3 tags")
					} else { // Second filter has minTags: 1
						require.Len(t, tags, 1, "Second filter should generate 1 tag")
					}

					// Check categories based on hasCategories filter
					categories, ok := blogPost["categories"].([]interface{})
					require.True(t, ok, "categories should be an array")
					if i < 2 { // First filter has hasCategories: true
						require.NotEmpty(t, categories, "First filter should have categories")
					} else { // Second filter has hasCategories: false
						require.Empty(t, categories, "Second filter should have no categories")
					}

					// Check nested lists
					tagGroups, ok := blogPost["tagGroups"].([]interface{})
					require.True(t, ok, "tagGroups should be an array")
					require.NotEmpty(t, tagGroups, "tagGroups should not be empty")

					// Check complex lists
					relatedCategories, ok := blogPost["relatedCategories"].([]interface{})
					require.True(t, ok, "relatedCategories should be an array")
					require.Len(t, relatedCategories, 1, "Each post should have 1 related category")

					contributors, ok := blogPost["contributors"].([]interface{})
					require.True(t, ok, "contributors should be an array")
					require.Len(t, contributors, 1, "Each post should have 1 contributor")

					// Check nested complex lists
					categoryGroups, ok := blogPost["categoryGroups"].([]interface{})
					require.True(t, ok, "categoryGroups should be an array")
					require.Len(t, categoryGroups, 1, "Each post should have 1 category group")
				}
			},
		},
		{
			name: "Should handle bulk create authors with nullable list parameter",
			query: `mutation($authors: [AuthorInput!]) {
				bulkCreateAuthors(authors: $authors) {
					id
					name
					email
					skills
					languages
					socialLinks
					teamsByProject
					collaborations
					favoriteCategories {
						id
						name
						kind
					}
					authorGroups {
						id
						name
					}
					projectTeams {
						id
						name
					}
				}
			}`,
			vars: `{
				"variables":
					{"authors":[
						{"name":"Bulk Author 1","email":"bulk1@example.com","skills":["Go","GraphQL"],"languages":["English","French"],"socialLinks":["github.com/bulk1"],"teamsByProject":[["Team1Member1","Team1Member2"]],"collaborations":[["Project1","Project2"]],"favoriteCategories":[{"name":"Programming","kind":"ELECTRONICS"}],"authorGroups":[[{"name":"GroupMember1"}]],"projectTeams":[[{"name":"TeamMember1"}]]},
						{"name":"Bulk Author 2","email":"bulk2@example.com","skills":["Python","REST"],"languages":["English","Spanish"],"teamsByProject":[["Team2Member1"]],"favoriteCategories":[{"name":"API Design","kind":"OTHER"}]}
					]}
				}
				`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkCreateAuthors, ok := data["bulkCreateAuthors"].([]interface{})
				require.True(t, ok, "bulkCreateAuthors should be an array")
				require.Len(t, bulkCreateAuthors, 2, "Should create 2 authors")

				for i, auth := range bulkCreateAuthors {
					author, ok := auth.(map[string]interface{})
					require.True(t, ok, "each author should be an object")
					require.NotEmpty(t, author["id"])
					require.Contains(t, author["id"].(string), "bulk-created-author")

					switch i {
					case 0:
						require.Equal(t, "Bulk Author 1", author["name"])
						require.Equal(t, "bulk1@example.com", author["email"])
						skills, ok := author["skills"].([]interface{})
						require.True(t, ok, "skills should be an array")
						require.Contains(t, skills, "Go")
						require.Contains(t, skills, "GraphQL")
					case 1:
						require.Equal(t, "Bulk Author 2", author["name"])
						require.Equal(t, "bulk2@example.com", author["email"])
						skills, ok := author["skills"].([]interface{})
						require.True(t, ok, "skills should be an array")
						require.Contains(t, skills, "Python")
						require.Contains(t, skills, "REST")
					}

					// Check nested lists
					teamsByProject, ok := author["teamsByProject"].([]interface{})
					require.True(t, ok, "teamsByProject should be an array")
					require.NotEmpty(t, teamsByProject, "teamsByProject should not be empty")

					// Check complex lists
					favoriteCategories, ok := author["favoriteCategories"].([]interface{})
					require.True(t, ok, "favoriteCategories should be an array")
					require.Len(t, favoriteCategories, 1, "Each author should have 1 favorite category")
				}
			},
		},
		{
			name: "Should handle bulk update authors with nullable list parameter",
			query: `mutation($authors: [AuthorInput!]) {
				bulkUpdateAuthors(authors: $authors) {
					id
					name
					email
					skills
					favoriteCategories {
						id
						name
						kind
					}
				}
			}`,
			vars: `{"variables":
				{"authors":[
					{"name":"Updated Author 1","email":"updated1@example.com","skills":["Rust","gRPC"],"languages":["English"], "teamsByProject":[["Team1Member1","Team1Member2"]],"favoriteCategories":[{"name":"Systems Programming","kind":"ELECTRONICS"}]},
					{"name":"Updated Author 2","email":"updated2@example.com","skills":["Python","REST"],"languages":["English","Spanish"], "teamsByProject":[["Team2Member1"]],"favoriteCategories":[{"name":"API Design","kind":"OTHER"}]}
				]}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkUpdateAuthors, ok := data["bulkUpdateAuthors"].([]interface{})
				require.True(t, ok, "bulkUpdateAuthors should be an array")
				require.Len(t, bulkUpdateAuthors, 2, "Should update 2 authors")

				author, ok := bulkUpdateAuthors[0].(map[string]interface{})
				require.True(t, ok, "author should be an object")
				require.NotEmpty(t, author["id"])
				require.Contains(t, author["id"].(string), "bulk-updated-author")
				require.Equal(t, "Updated Author 1", author["name"])
				require.Equal(t, "updated1@example.com", author["email"])

				skills, ok := author["skills"].([]interface{})
				require.True(t, ok, "skills should be an array")
				require.Contains(t, skills, "Rust")
				require.Contains(t, skills, "gRPC")

				favoriteCategories, ok := author["favoriteCategories"].([]interface{})
				require.True(t, ok, "favoriteCategories should be an array")
				require.Len(t, favoriteCategories, 1, "Should have 1 favorite category")

				author, ok = bulkUpdateAuthors[1].(map[string]interface{})
				require.True(t, ok, "author should be an object")
				require.NotEmpty(t, author["id"])
				require.Contains(t, author["id"].(string), "bulk-updated-author")
				require.Equal(t, "Updated Author 2", author["name"])
				require.Equal(t, "updated2@example.com", author["email"])

				skills, ok = author["skills"].([]interface{})
				require.True(t, ok, "skills should be an array")
				require.Contains(t, skills, "Python")
				require.Contains(t, skills, "REST")

				favoriteCategories, ok = author["favoriteCategories"].([]interface{})
				require.True(t, ok, "favoriteCategories should be an array")
				require.Len(t, favoriteCategories, 1, "Should have 1 favorite category")
			},
		},
		{
			name: "Should handle bulk create blog posts with nullable list parameter",
			query: `mutation($blogPosts: [BlogPostInput!]) {
				bulkCreateBlogPosts(blogPosts: $blogPosts) {
					id
					title
					content
					tags
					optionalTags
					categories
					keywords
					viewCounts
					ratings
					isPublished
					tagGroups
					relatedTopics
					commentThreads
					suggestions
					relatedCategories {
						id
						name
						kind
					}
					contributors {
						id
						name
					}
					categoryGroups {
						id
						name
						kind
					}
				}
			}`,
			vars: `{"variables":{"blogPosts":[{"title":"Bulk Post 1","content":"Content for bulk post 1","tags":["bulk","test"],"optionalTags":["optional1"],"categories":["Technology","Testing"],"keywords":["bulk","creation"],"viewCounts":[100,200],"ratings":[4.5,5.0],"isPublished":[true,false],"tagGroups":[["bulk","tags"],["test","creation"]],"relatedTopics":[["bulk","operations"],["testing","mutations"]],"commentThreads":[["Great bulk feature!","Very useful"],["Testing works well"]],"suggestions":[["Add more examples"],["Improve documentation"]],"relatedCategories":[{"name":"Bulk Operations","kind":"ELECTRONICS"}],"contributors":[{"name":"Bulk Creator"}],"categoryGroups":[[{"name":"Bulk Category","kind":"OTHER"}]]},{"title":"Bulk Post 2","content":"Content for bulk post 2","tags":["second","bulk"],"categories":["Development"],"viewCounts":[150],"tagGroups":[["second","post"]],"relatedTopics":[["development"]],"commentThreads":[["Second post!"]]}]}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkCreateBlogPosts, ok := data["bulkCreateBlogPosts"].([]interface{})
				require.True(t, ok, "bulkCreateBlogPosts should be an array")
				require.Len(t, bulkCreateBlogPosts, 2, "Should create 2 blog posts")

				for i, post := range bulkCreateBlogPosts {
					blogPost, ok := post.(map[string]interface{})
					require.True(t, ok, "each blog post should be an object")
					require.NotEmpty(t, blogPost["id"])
					require.Contains(t, blogPost["id"].(string), "bulk-created-post")

					switch i {
					case 0:
						require.Equal(t, "Bulk Post 1", blogPost["title"])
						require.Equal(t, "Content for bulk post 1", blogPost["content"])
						tags, ok := blogPost["tags"].([]interface{})
						require.True(t, ok, "tags should be an array")
						require.Contains(t, tags, "bulk")
						require.Contains(t, tags, "test")

						optionalTags, ok := blogPost["optionalTags"].([]interface{})
						require.True(t, ok, "optionalTags should be an array")
						require.Contains(t, optionalTags, "optional1")
					case 1:
						require.Equal(t, "Bulk Post 2", blogPost["title"])
						require.Equal(t, "Content for bulk post 2", blogPost["content"])
						tags, ok := blogPost["tags"].([]interface{})
						require.True(t, ok, "tags should be an array")
						require.Contains(t, tags, "second")
						require.Contains(t, tags, "bulk")
					}

					// Check nested lists
					tagGroups, ok := blogPost["tagGroups"].([]interface{})
					require.True(t, ok, "tagGroups should be an array")
					require.NotEmpty(t, tagGroups, "tagGroups should not be empty")

					relatedTopics, ok := blogPost["relatedTopics"].([]interface{})
					require.True(t, ok, "relatedTopics should be an array")
					require.NotEmpty(t, relatedTopics, "relatedTopics should not be empty")

					commentThreads, ok := blogPost["commentThreads"].([]interface{})
					require.True(t, ok, "commentThreads should be an array")
					require.NotEmpty(t, commentThreads, "commentThreads should not be empty")
				}
			},
		},
		{
			name: "Should handle bulk update blog posts with nullable list parameter",
			query: `mutation($blogPosts: [BlogPostInput!]) {
				bulkUpdateBlogPosts(blogPosts: $blogPosts) {
					id
					title
					content
					tags
					categories
					viewCounts
					tagGroups
				}
			}`,
			vars: `{"variables":{"blogPosts":[
				{
					"title":"Updated Bulk Post",
					"content":"Updated content",
					"tags":["updated","bulk","post"],
					"categories":["Updated Technology"],
					"viewCounts":[300,400,500],
					"tagGroups":[["updated","tags"],["bulk","update"]],
					"commentThreads":[["Updated comment"]],
					"relatedTopics":[["updated","topics"]],
				}
			]}}`,
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkUpdateBlogPosts, ok := data["bulkUpdateBlogPosts"].([]interface{})
				require.True(t, ok, "bulkUpdateBlogPosts should be an array")
				require.Len(t, bulkUpdateBlogPosts, 1, "Should update 1 blog post")

				blogPost, ok := bulkUpdateBlogPosts[0].(map[string]interface{})
				require.True(t, ok, "blog post should be an object")
				require.NotEmpty(t, blogPost["id"])
				require.Contains(t, blogPost["id"].(string), "bulk-updated-post")
				require.Equal(t, "Updated Bulk Post", blogPost["title"])
				require.Equal(t, "Updated content", blogPost["content"])

				tags, ok := blogPost["tags"].([]interface{})
				require.True(t, ok, "tags should be an array")
				require.Contains(t, tags, "updated")
				require.Contains(t, tags, "bulk")
				require.Contains(t, tags, "post")

				categories, ok := blogPost["categories"].([]interface{})
				require.True(t, ok, "categories should be an array")
				require.Contains(t, categories, "Updated Technology")

				viewCounts, ok := blogPost["viewCounts"].([]interface{})
				require.True(t, ok, "viewCounts should be an array")
				require.Contains(t, viewCounts, float64(300))
				require.Contains(t, viewCounts, float64(400))
				require.Contains(t, viewCounts, float64(500))
			},
		},
		{
			name: "Should handle bulk operations with empty nullable lists",
			query: `query {
				bulkSearchAuthors(filters: []) {
					id
					name
				}
				bulkSearchBlogPosts(filters: []) {
					id
					title
				}
			}`,
			vars: "{}",
			validate: func(t *testing.T, data map[string]interface{}) {
				bulkSearchAuthors, ok := data["bulkSearchAuthors"].([]interface{})
				require.True(t, ok, "bulkSearchAuthors should be an array")
				require.Empty(t, bulkSearchAuthors, "Should return empty array when filters is empty")

				bulkSearchBlogPosts, ok := data["bulkSearchBlogPosts"].([]interface{})
				require.True(t, ok, "bulkSearchBlogPosts should be an array")
				require.Empty(t, bulkSearchBlogPosts, "Should return empty array when filters is empty")
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
