package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

// TestNewProtoCompiler tests the basic functionality of the Proto compiler
// It verifies that the compiler can successfully parse a valid protobuf definition
func TestNewProtoCompiler(t *testing.T) {
	// Create a new compiler with the valid protobuf definition
	compiler, err := NewProtoCompiler(validProto)
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	require.Equal(t, "product.v1", compiler.doc.Package)

	// At this point, compiler.doc should contain all the services, methods, and messages
	// defined in the protobuf definition
}

// TestBuildProtoMessage tests the ability to build a protobuf message
// from an execution plan and JSON data
func TestBuildProtoMessage(t *testing.T) {
	// Create and parse the protobuf definition
	compiler, err := NewProtoCompiler(validProto)
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
							JSONPath: "variables.representations", // Path to extract data from GraphQL variables
							Index:    1,
							Message: &RPCMessage{
								Name: "LookupProductByIdInput",
								Fields: []RPCField{
									{
										Name:     "key",
										TypeName: string(DataTypeMessage),
										Index:    1,
										Message: &RPCMessage{
											Name: "ProductByIdKey",
											Fields: []RPCField{
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id", // Extract 'id' from each representation
													Index:    1,
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
							Index:    1,
							Message: &RPCMessage{
								Name: "LookupProductByIdResult",
								Fields: []RPCField{
									{
										Name:     "product",
										TypeName: string(DataTypeMessage),
										Index:    1,
										Message: &RPCMessage{
											Name: "Product",
											Fields: []RPCField{
												{
													Name:     "id",
													TypeName: string(DataTypeString),
													JSONPath: "id",
													Index:    1,
												},
												{
													Name:     "name",
													TypeName: string(DataTypeString),
													JSONPath: "name",
													Index:    2,
												},
												{
													Name:     "price",
													TypeName: string(DataTypeDouble),
													JSONPath: "price",
													Index:    3,
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
