package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	// 1. Start a real gRPC server with our mock implementation
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

	// Clean up the server when the test completes
	defer server.Stop()

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
	)

	require.NoError(t, err)
	defer conn.Close()

	// 3. Set up GraphQL query and schema
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

	// 4. Create a datasource with the real gRPC client connection
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

	// 5. Execute the query through our datasource
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
	// 1. Start a real gRPC server with our mock implementation
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

	// Clean up the server when the test completes
	defer server.Stop()

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
	defer conn.Close()

	// 3. Set up GraphQL query and schema
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

	// 4. Create a datasource with the real gRPC client connection
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

	// 5. Execute the query through our datasource
	output := new(bytes.Buffer)

	// Format the input with query and variables
	inputJSON := fmt.Sprintf(`{"query":%q,"body":%s}`, query, variables)
	t.Logf("Input JSON: %s", inputJSON)

	err = ds.Load(context.Background(), []byte(inputJSON), output)
	require.NoError(t, err)

	// Print the response for debugging
	responseData := output.String()
	t.Logf("Response: %s", responseData)

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
	// 1. Start a gRPC server with our mock implementation
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Get the server address
	serverAddr := fmt.Sprintf("localhost:%d", lis.Addr().(*net.TCPAddr).Port)

	// Create and start the gRPC server
	server := grpc.NewServer()
	mockService := &grpctest.MockService{}
	productv1.RegisterProductServiceServer(server, mockService)

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()
	defer server.Stop()

	// 2. Connect to the gRPC server
	// see https://github.com/grpc/grpc-go/issues/7091
	// nolint: staticcheck
	conn, err := grpc.Dial(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithLocalDNSResolution(),
	)
	require.NoError(t, err)
	defer conn.Close()

	// 3. Set up the GraphQL query that will trigger the error
	query := `query UserQuery($id: ID!) { user(id: $id) { id name } }`
	variables := `{"variables":{"id":"error-user"}}`

	// 4. Parse the schema and query
	schemaDoc := grpctest.MustGraphQLSchema(t)

	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// 5. Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Compiler:     compiler,
	})
	require.NoError(t, err)

	// 6. Execute the query
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","body":`+variables+`}`), output)
	require.NoError(t, err, "Load should not return an error even when the gRPC call fails")

	// 7. Print response for debugging
	responseJson := output.String()
	t.Logf("Error Response: %s", responseJson)

	// 8. Verify the response format according to GraphQL specification
	// The response should have an "errors" array with the error message
	require.Contains(t, responseJson, "errors")
	require.Contains(t, responseJson, "user not found: error-user")

	// 9. Parse the response JSON for more detailed validation
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
				Name:     "results",
				TypeName: string(DataTypeMessage),
				Repeated: true,
				Index:    0,
				JSONPath: "results",
				Message: &RPCMessage{
					Name: "LookupProductByIdResult",
					Fields: []RPCField{
						{
							Name:     "product",
							TypeName: string(DataTypeMessage),
							Index:    0,
							JSONPath: "product",
							Message: &RPCMessage{
								Name: "Product",
								Fields: []RPCField{
									{
										Name:     "id",
										TypeName: string(DataTypeString),
										JSONPath: "id",
										Index:    0,
									},
									{
										Name:     "name",
										TypeName: string(DataTypeString),
										JSONPath: "name_different",
										Index:    1,
									},
									{
										Name:     "price",
										TypeName: string(DataTypeDouble),
										JSONPath: "price_different",
										Index:    2,
									},
								},
							},
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

	resultMessageDesc := compiler.doc.MessageByName("LookupProductByIdResult").Desc
	resultMessage := dynamicpb.NewMessage(resultMessageDesc)
	resultMessage.Set(resultMessageDesc.Fields().ByName("product"), protoref.ValueOfMessage(productMessage))

	responseMessageDesc := compiler.doc.MessageByName("LookupProductByIdResponse").Desc
	responseMessage := dynamicpb.NewMessage(responseMessageDesc)
	responseMessage.Mutable(responseMessageDesc.Fields().ByName("results")).List().Append(protoref.ValueOfMessage(resultMessage))

	ds := &DataSource{}

	arena := astjson.Arena{}
	responseJSON, err := ds.marshalResponseJSON(&arena, &response, responseMessage)
	require.NoError(t, err)
	require.Equal(t, `{"results":[{"product":{"id":"123","name_different":"test","price_different":123.45}}]}`, responseJSON.String())
}

// TODO test interface types
// Test_DataSource_Load_WithAnimalInterface tests the datasource with Animal interface types (Cat/Dog)
// using a bufconn connection to the mock service
// func Test_DataSource_Load_WithAnimalInterface(t *testing.T) {
// 	// Set up the bufconn listener
// 	lis := bufconn.Listen(1024 * 1024)

// 	// Create a new gRPC server
// 	server := grpc.NewServer()

// 	// Register our mock service implementation
// 	mockService := &grpctest.MockService{}
// 	productv1.RegisterProductServiceServer(server, mockService)

// 	// Start the server in a goroutine
// 	go func() {
// 		if err := server.Serve(lis); err != nil {
// 			t.Errorf("failed to serve: %v", err)
// 		}
// 	}()

// 	// Clean up the server when the test completes
// 	defer server.Stop()

// 	// Create a buffer-based dialer
// 	bufDialer := func(context.Context, string) (net.Conn, error) {
// 		return lis.Dial()
// 	}

// 	// Connect using bufconn dialer
// 	conn, err := grpc.Dial(
// 		"bufnet",
// 		grpc.WithTransportCredentials(insecure.NewCredentials()),
// 		grpc.WithContextDialer(bufDialer),
// 	)
// 	require.NoError(t, err)
// 	defer conn.Close()

// 	// Define the GraphQL query for the Animal interface
// 	query := `query RandomPetQuery {
// 		randomPet {
// 			id
// 			name
// 			kind
// 			... on Cat {
// 				meowVolume
// 			}
// 			... on Dog {
// 				barkVolume
// 			}
// 		}
// 	}`

// 	report := &operationreport.Report{}

// 	// Parse the GraphQL schema
// 	schemaDoc := ast.NewDocument()
// 	schemaDoc.Input.ResetInputString(string(grpctest.MustGraphQLSchema(t).RawSchema()))
// 	astparser.NewParser().Parse(schemaDoc, report)
// 	require.False(t, report.HasErrors(), "failed to parse schema: %s", report.Error())

// 	// Parse the GraphQL query
// 	queryDoc := ast.NewDocument()
// 	queryDoc.Input.ResetInputString(query)
// 	astparser.NewParser().Parse(queryDoc, report)
// 	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

// 	// Transform the GraphQL ASTs
// 	err = asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
// 	require.NoError(t, err, "failed to merge schema with base")

// 	// Create mapping configuration based on the mapping.go
// 	mapping := &GRPCMapping{
// 		Service: "ProductService",
// 		QueryRPCs: map[string]RPCConfig{
// 			"randomPet": {
// 				RPC:      "QueryRandomPet",
// 				Request:  "QueryRandomPetRequest",
// 				Response: "QueryRandomPetResponse",
// 			},
// 		},
// 		Fields: map[string]FieldMap{
// 			"Query": {
// 				"randomPet": {
// 					TargetName: "random_pet",
// 				},
// 			},
// 			"Cat": {
// 				"id": {
// 					TargetName: "id",
// 				},
// 				"name": {
// 					TargetName: "name",
// 				},
// 				"kind": {
// 					TargetName: "kind",
// 				},
// 				"meowVolume": {
// 					TargetName: "meow_volume",
// 				},
// 			},
// 			"Dog": {
// 				"id": {
// 					TargetName: "id",
// 				},
// 				"name": {
// 					TargetName: "name",
// 				},
// 				"kind": {
// 					TargetName: "kind",
// 				},
// 				"barkVolume": {
// 					TargetName: "bark_volume",
// 				},
// 			},
// 		},
// 	}

// 	// Create the datasource
// 	ds, err := NewDataSource(conn, DataSourceConfig{
// 		Operation:    queryDoc,
// 		Definition:   schemaDoc,
// 		ProtoSchema:  grpctest.MustProtoSchema(t),
// 		SubgraphName: "Products",
// 		Mapping:      mapping,
// 	})
// 	require.NoError(t, err)

// 	// Execute the query through our datasource
// 	output := new(bytes.Buffer)
// 	err = ds.Load(context.Background(), []byte(`{"query":`+fmt.Sprintf("%q", query)+`}`), output)
// 	require.NoError(t, err)

// 	// Print the response for debugging
// 	responseData := output.String()
// 	t.Logf("Response: %s", responseData)

// 	// Define a response structure that can handle both Cat and Dog types
// 	type response struct {
// 		Data struct {
// 			RandomPet map[string]interface{} `json:"randomPet"`
// 		} `json:"data"`
// 		Errors []struct {
// 			Message string `json:"message"`
// 		} `json:"errors,omitempty"`
// 	}

// 	var resp response
// 	err = json.Unmarshal(output.Bytes(), &resp)
// 	require.NoError(t, err, "Failed to unmarshal response")

// 	// Verify there are no errors
// 	require.Empty(t, resp.Errors, "Response should not contain errors")

// 	// Verify we have data
// 	require.NotNil(t, resp.Data.RandomPet, "RandomPet should not be nil")

// 	// Check if we got either a cat or dog by checking for their specific fields
// 	if _, hasCat := resp.Data.RandomPet["meowVolume"]; hasCat {
// 		// We got a Cat response
// 		require.Contains(t, resp.Data.RandomPet, "id")
// 		require.Contains(t, resp.Data.RandomPet, "name")
// 		require.Contains(t, resp.Data.RandomPet, "kind")
// 		require.Contains(t, resp.Data.RandomPet, "meowVolume")
// 	} else if _, hasDog := resp.Data.RandomPet["barkVolume"]; hasDog {
// 		// We got a Dog response
// 		require.Contains(t, resp.Data.RandomPet, "id")
// 		require.Contains(t, resp.Data.RandomPet, "name")
// 		require.Contains(t, resp.Data.RandomPet, "kind")
// 		require.Contains(t, resp.Data.RandomPet, "barkVolume")
// 	} else {
// 		t.Fatalf("Response doesn't contain either a Cat or Dog type: %v", resp.Data.RandomPet)
// 	}
// }

// Test_DataSource_Load_WithProductQueries tests the product-related query operations
func Test_DataSource_Load_WithCategoryQueries(t *testing.T) {
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

	// Clean up the server when the test completes
	defer server.Stop()

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
	defer conn.Close()

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

			// Create a new GRPCMapping configuration
			mapping := &GRPCMapping{
				Service: "ProductService",
				QueryRPCs: map[string]RPCConfig{
					"categories": {
						RPC:      "QueryCategories",
						Request:  "QueryCategoriesRequest",
						Response: "QueryCategoriesResponse",
					},
					"categoriesByKind": {
						RPC:      "QueryCategoriesByKind",
						Request:  "QueryCategoriesByKindRequest",
						Response: "QueryCategoriesByKindResponse",
					},
					"filterCategories": {
						RPC:      "QueryFilterCategories",
						Request:  "QueryFilterCategoriesRequest",
						Response: "QueryFilterCategoriesResponse",
					},
				},
				EnumValues: map[string][]EnumValueMapping{
					"CategoryKind": {
						{Value: "BOOK", TargetValue: "CATEGORY_KIND_BOOK"},
						{Value: "ELECTRONICS", TargetValue: "CATEGORY_KIND_ELECTRONICS"},
						{Value: "FURNITURE", TargetValue: "CATEGORY_KIND_FURNITURE"},
						{Value: "OTHER", TargetValue: "CATEGORY_KIND_OTHER"},
					},
				},
				Fields: map[string]FieldMap{
					"Query": {
						"categories": {
							TargetName: "categories",
						},
						"categoriesByKind": {
							TargetName: "categories_by_kind",
							ArgumentMappings: map[string]string{
								"kind": "kind",
							},
						},
						"filterCategories": {
							TargetName: "filter_categories",
							ArgumentMappings: map[string]string{
								"filter": "filter",
							},
						},
					},
					"Category": {
						"id": {
							TargetName: "id",
						},
						"name": {
							TargetName: "name",
						},
						"kind": {
							TargetName: "kind",
						},
					},
					"CategoryFilter": {
						"category": {
							TargetName: "category",
						},
						"pagination": {
							TargetName: "pagination",
						},
					},
					"Pagination": {
						"page": {
							TargetName: "page",
						},
						"perPage": {
							TargetName: "per_page",
						},
					},
				},
			}

			compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
			if err != nil {
				t.Fatalf("failed to compile proto: %v", err)
			}

			// Create the datasource
			ds, err := NewDataSource(conn, DataSourceConfig{
				Operation:    &queryDoc,
				Definition:   &schemaDoc,
				SubgraphName: "Products",
				Mapping:      mapping,
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

// Test_DataSource_Load_WithTypename tests that __typename fields are correctly included
// in the response with their static values
func Test_DataSource_Load_WithTypename(t *testing.T) {
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

	// Clean up the server when the test completes
	defer server.Stop()

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
	defer conn.Close()

	// Define GraphQL query that requests __typename
	query := `query UsersWithTypename { users { __typename id name } }`

	// Parse the GraphQL schema
	schemaDoc := grpctest.MustGraphQLSchema(t)

	// Parse the GraphQL query
	queryDoc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	// Create a new GRPCMapping configuration
	mapping := &GRPCMapping{
		Service: "ProductService",
		QueryRPCs: map[string]RPCConfig{
			"users": {
				RPC:      "QueryUsers",
				Request:  "QueryUsersRequest",
				Response: "QueryUsersResponse",
			},
		},
		Fields: map[string]FieldMap{
			"Query": {
				"users": {
					TargetName: "users",
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
	}

	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    &queryDoc,
		Definition:   &schemaDoc,
		SubgraphName: "Products",
		Mapping:      mapping,
		Compiler:     compiler,
	})
	require.NoError(t, err)

	// Execute the query through our datasource
	output := new(bytes.Buffer)
	input := fmt.Sprintf(`{"query":%q,"body":{}}`, query)
	err = ds.Load(context.Background(), []byte(input), output)
	require.NoError(t, err)

	// Log the response for debugging
	t.Logf("Response: %s", output.String())

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
