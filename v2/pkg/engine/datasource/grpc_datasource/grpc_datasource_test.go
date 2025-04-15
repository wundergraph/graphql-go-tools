package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata/productv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
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
	if method == "QueryComplexFilterType" {
		// Populate the response with test data using protojson.Unmarshal
		responseJSON := []byte(`{"typeWithComplexFilterInput":[{"id":"test-id-123", "name":"Test Product"}]}`)
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
			TypeWithComplexFilterInput []struct {
				Id   string `json:"id"`
				Name string `json:"name"`
			} `json:"typeWithComplexFilterInput"`
		} `json:"data"`
	}

	var resp response

	bytes := output.Bytes()
	fmt.Println(string(bytes))

	err = json.Unmarshal(bytes, &resp)
	require.NoError(t, err)

	require.Equal(t, resp.Data.TypeWithComplexFilterInput[0].Id, "test-id-123")
	require.Equal(t, resp.Data.TypeWithComplexFilterInput[0].Name, "Test Product")
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
