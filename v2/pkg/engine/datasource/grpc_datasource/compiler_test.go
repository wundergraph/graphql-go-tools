package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
)

// Complete valid protobuf definition with service and message definitions
// This simulates a product service with methods to look up products by ID or name
var validProto = `
syntax = "proto3";
package product.v1;

option go_package = "grpc-graphql/pkg/proto/product/v1;productv1";

service ProductService {
  rpc LookupProductById(LookupProductByIdRequest) returns (LookupProductByIdResponse) {}
  rpc LookupProductByName(LookupProductByNameRequest) returns (LookupProductByNameResponse) {}
}

message LookupProductByNameRequest {
  repeated LookupProductByNameInput inputs = 1;
}

message LookupProductByNameInput {
  string name = 1;
}

message LookupProductByNameResponse {
  repeated LookupProductByNameResult results = 1;
}

message LookupProductByNameResult {
  Product product = 1;
}

message LookupProductByIdRequest {
  repeated LookupProductByIdInput inputs = 1;
}

message LookupProductByIdInput {
  ProductByIdKey key = 1;
}

message ProductByIdKey {
  string id = 1;
}

message LookupProductByIdResponse {
  repeated LookupProductByIdResult results = 1;
}

message LookupProductByIdResult {
  Product product = 1;
}

message Product {
  string id = 1;
  string name = 2;
  double price = 3;
}
`

var invalidProtoMissingResponseDefintition = `
syntax = "proto3";
package product.v1;

option go_package = "grpc-graphql/pkg/proto/product/v1;productv1";

service ProductService {
  rpc LookupProductById(LookupProductByIdRequest) returns (LookupProductByIdResponse) {}
}

message LookupProductByIdRequest {
  repeated LookupProductByIdInput inputs = 1;
}

message LookupProductByIdInput {
  ProductByIdKey key = 1;
}

message ProductByIdKey {
  string id = 1;
}
`

var protoSchemaWithRecursiveType = `
syntax = "proto3";
package product.v1;

option go_package = "grpc-graphql/pkg/proto/product/v1;productv1";

message RecursiveMessage {
  string id = 1;
  RecursiveMessage nested = 2;
}
`

// TestNewProtoCompiler tests the basic functionality of the Proto compiler
// It verifies that the compiler can successfully parse a valid protobuf definition
func TestNewProtoCompiler(t *testing.T) {
	// Create a new compiler with the valid protobuf definition
	compiler, err := NewProtoCompiler(validProto, nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	require.Equal(t, "product.v1", compiler.doc.Package)

	// At this point, compiler.doc should contain all the services, methods, and messages
	// defined in the protobuf definition
}

func TestNewProtoCompilerInvalid(t *testing.T) {
	compiler, err := NewProtoCompiler(invalidProtoMissingResponseDefintition, nil)
	require.ErrorContains(t, err, "unknown response type LookupProductByIdResponse")
	require.Nil(t, compiler)
}

func TestNewProtoCompilerRecursiveType(t *testing.T) {
	compiler, err := NewProtoCompiler(protoSchemaWithRecursiveType, nil)
	require.NoError(t, err)
	require.Equal(t, "product.v1", compiler.doc.Package)
	require.Equal(t, 1, len(compiler.doc.Messages))
	require.Equal(t, "RecursiveMessage", compiler.doc.Messages[0].Name)
	require.Equal(t, 2, len(compiler.doc.Messages[0].Fields))
	require.Equal(t, "nested", compiler.doc.Messages[0].Fields[1].Name)
	require.Equal(t, "RecursiveMessage", compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc).Name)
	require.Equal(t, 2, len(compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc).Fields))
	require.Equal(t, "id", compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc).Fields[0].Name)
	require.Equal(t, "nested", compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc).Fields[1].Name)
	require.Equal(t, "RecursiveMessage", compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc).Fields[1].ResolveUnderlyingMessage(compiler.doc).Name)
}

func TestNewProtoCompilerNestedRecursiveType(t *testing.T) {
	protoSchemaWithNestedRecursiveType := `
syntax = "proto3";
package product.v1;

option go_package = "grpc-graphql/pkg/proto/product/v1;productv1";

message NestedRecursiveMessage {
  string id = 1;
  RecursiveMessage nested = 2;
}

message RecursiveMessage {
  string id = 1;
  NestedRecursiveMessage nested = 2;
}
`
	compiler, err := NewProtoCompiler(protoSchemaWithNestedRecursiveType, nil)

	require.NoError(t, err)
	require.Equal(t, "product.v1", compiler.doc.Package)
	require.Equal(t, 2, len(compiler.doc.Messages))

	require.Equal(t, "NestedRecursiveMessage", compiler.doc.Messages[0].Name)
	require.Equal(t, 2, len(compiler.doc.Messages[0].Fields))
	require.Equal(t, "id", compiler.doc.Messages[0].Fields[0].Name)
	require.Equal(t, "nested", compiler.doc.Messages[0].Fields[1].Name)

	nested := compiler.doc.Messages[0].Fields[1].ResolveUnderlyingMessage(compiler.doc)
	require.Equal(t, "RecursiveMessage", nested.Name)

	require.Equal(t, 2, len(nested.Fields))
	require.Equal(t, "id", nested.Fields[0].Name)
	require.Equal(t, "nested", nested.Fields[1].Name)

	nested = nested.Fields[1].ResolveUnderlyingMessage(compiler.doc)
	require.Equal(t, "NestedRecursiveMessage", nested.Name)

	require.Equal(t, 2, len(nested.Fields))
	require.Equal(t, "id", nested.Fields[0].Name)
	require.Equal(t, "nested", nested.Fields[1].Name)

	nested = nested.Fields[1].ResolveUnderlyingMessage(compiler.doc)
	require.Equal(t, "RecursiveMessage", nested.Name)

	require.Equal(t, 2, len(nested.Fields))
	require.Equal(t, "id", nested.Fields[0].Name)
	require.Equal(t, "nested", nested.Fields[1].Name)
	require.Equal(t, "NestedRecursiveMessage", nested.Fields[1].ResolveUnderlyingMessage(compiler.doc).Name)
}

// TestBuildProtoMessage tests the ability to build a protobuf message
// from an execution plan and JSON data
func TestBuildProtoMessage(t *testing.T) {
	// Create and parse the protobuf definition
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), nil)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	// Create an execution plan that defines how to build the protobuf message
	// This plan describes how to call the LookupProductById method
	executionPlan := &RPCExecutionPlan{
		Calls: []RPCCall{
			{
				ServiceName: "ProductService",
				MethodName:  "LookupProductById",
				// Define the structure of the request message
				Request: RPCMessage{
					Name: "LookupProductByIdRequest",
					Fields: []RPCField{
						{
							Name:     "inputs",
							TypeName: string(DataTypeMessage),
							Repeated: true,
							JSONPath: "representations", // Path to extract data from GraphQL variables
							Message: &RPCMessage{
								Name: "LookupProductByIdInput",
								Fields: []RPCField{
									{
										Name:     "key",
										TypeName: string(DataTypeMessage),
										Message: &RPCMessage{
											Name: "ProductByIdKey",
											Fields: []RPCField{
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id", // Extract 'id' from each representation
												},
											},
										},
									},
								},
							},
						},
					},
				},
				// Define the structure of the response message
				Response: RPCMessage{
					Name: "LookupProductByIdResponse",
					Fields: []RPCField{
						{
							Name:     "results",
							TypeName: string(DataTypeMessage),
							Repeated: true,
							JSONPath: "results",
							Message: &RPCMessage{
								Name: "LookupProductByIdResult",
								Fields: []RPCField{
									{
										Name:     "product",
										TypeName: string(DataTypeMessage),
										Message: &RPCMessage{
											Name: "Product",
											Fields: []RPCField{
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Sample GraphQL variables containing a product representation
	variables := []byte(`{"variables":{"representations":[{"__typename":"Product","id":"123"}]}}`)

	// Compile the execution plan with the variables
	// This should build a protobuf message ready to be sent to the gRPC service
	invocations, err := compiler.Compile(executionPlan, gjson.ParseBytes(variables))
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	require.Equal(t, 1, len(invocations))

}

func TestCompileNestedLists(t *testing.T) {
	compiler, err := NewProtoCompiler(grpctest.MustProtoSchema(t), testMapping())
	require.NoError(t, err)

	require.Equal(t, "productv1", compiler.doc.Package)

	listOfListOfString := compiler.doc.MessageByName("ListOfListOfString")
	require.Equal(t, "ListOfListOfString", listOfListOfString.Name)
	require.Equal(t, 1, len(listOfListOfString.Fields))
	require.Equal(t, "list", listOfListOfString.Fields[0].Name)

	message := compiler.doc.MessageByName("BlogPost")
	require.Equal(t, "BlogPost", message.Name)
	require.Equal(t, 2, len(message.Fields))
	require.Equal(t, "id", message.Fields[0].Name)
	require.Equal(t, "tag_groups", message.Fields[1].Name)
}
