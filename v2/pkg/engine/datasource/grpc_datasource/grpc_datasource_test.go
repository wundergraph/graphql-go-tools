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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata/productv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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

	report := &operationreport.Report{}
	// Parse the GraphQL schema
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse schema: %s", report.Error())
	}

	// Parse the GraphQL query
	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}
	// Transform the GraphQL ASTs
	err := asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	if err != nil {
		t.Fatalf("failed to merge schema with base: %s", err)
	}

	mi := mockInterface{}
	ds, err := NewDataSource(mi, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})

	require.NoError(t, err)

	output := new(bytes.Buffer)

	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
	require.NoError(t, err)

	fmt.Println(output.String())
}

// Test_DataSource_Load_WithMockService tests the datasource.Load method with an actual gRPC server
func Test_DataSource_Load_WithMockService(t *testing.T) {
	// 1. Start a real gRPC server with our mock implementation
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Get the assigned port
	port := lis.Addr().(*net.TCPAddr).Port
	serverAddr := fmt.Sprintf("localhost:%d", port)

	// Create a new gRPC server
	server := grpc.NewServer()

	// Register our mock service implementation
	mockService := &testdata.MockService{}
	productv1.RegisterProductServiceServer(server, mockService)

	// Start the server in a goroutine
	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	// Clean up the server when the test completes
	defer server.Stop()

	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	// 3. Set up GraphQL query and schema
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"Test Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	report := &operationreport.Report{}

	// Parse the GraphQL schema
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse schema: %s", report.Error())
	}

	// Parse the GraphQL query
	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	// Transform the GraphQL ASTs
	err = asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	if err != nil {
		t.Fatalf("failed to merge schema with base: %s", err)
	}

	// 4. Create a datasource with the real gRPC client connection
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})
	require.NoError(t, err)

	// 5. Execute the query through our datasource
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
	require.NoError(t, err)

	// Print the response for debugging
	fmt.Println(output.String())

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
	mockService := &testdata.MockService{}
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
	conn, err := grpc.Dial(
		"bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(bufDialer),
	)
	require.NoError(t, err)
	defer conn.Close()

	// 3. Set up GraphQL query and schema
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"HARDCODED_NAME_TEST","filterField1":"value1","filterField2":"value2"}}}}`

	report := &operationreport.Report{}

	// Parse the GraphQL schema
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse schema: %s", report.Error())
	}

	// Parse the GraphQL query
	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}

	// Transform the GraphQL ASTs
	err = asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	if err != nil {
		t.Fatalf("failed to merge schema with base: %s", err)
	}

	// 4. Create a datasource with the real gRPC client connection
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})
	require.NoError(t, err)

	// 5. Execute the query through our datasource
	output := new(bytes.Buffer)

	// Format the input with query and variables
	inputJSON := fmt.Sprintf(`{"query":%q,"variables":%s}`, query, variables)
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
	mockService := &testdata.MockService{}
	productv1.RegisterProductServiceServer(server, mockService)

	go func() {
		if err := server.Serve(lis); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()
	defer server.Stop()

	// 2. Connect to the gRPC server
	conn, err := grpc.NewClient(
		serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer conn.Close()

	// 3. Set up the GraphQL query that will trigger the error
	query := `query UserQuery($id: ID!) { user(id: $id) { id name } }`
	variables := `{"variables":{"id":"error-user"}}`

	// 4. Parse the schema and query
	report := &operationreport.Report{}
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	require.False(t, report.HasErrors(), "failed to parse schema: %s", report.Error())

	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	err = asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	require.NoError(t, err, "failed to merge schema with base")

	// 5. Create the datasource
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})
	require.NoError(t, err)

	// 6. Execute the query
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
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

	compiler, err := NewProtoCompiler(testdata.ProtoSchema(t))
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

	arena := astjson.Arena{}
	responseJSON := marshalResponseJSON(&arena, &response, responseMessage)
	require.Equal(t, `{"results":[{"product":{"id":"123","name_different":"test","price_different":123.45}}]}`, responseJSON.String())
}
