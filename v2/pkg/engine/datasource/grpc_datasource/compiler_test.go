package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
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
	require.Len(t, compiler.doc.Methods, 2)
	method := compiler.doc.Methods[0]
	require.Equal(t, "LookupProductById", method.Name)
	require.Equal(t, "LookupProductByIdRequest", compiler.doc.Messages[method.InputRef].Name)
	require.Equal(t, "LookupProductByIdResponse", compiler.doc.Messages[method.OutputRef].Name)
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

	message := compiler.runtime.getMessageByName("RecursiveMessage")
	require.NotNil(t, message)
	require.Equal(t, 2, len(message.fieldsByName))
	require.Contains(t, message.fieldsByName, "id")

	nested := message.fieldsByName["nested"].message
	require.Same(t, message, nested)
	require.Same(t, message, nested.fieldsByName["nested"].message)
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

	outer := compiler.runtime.getMessageByName("NestedRecursiveMessage")
	inner := compiler.runtime.getMessageByName("RecursiveMessage")
	require.NotNil(t, outer)
	require.NotNil(t, inner)

	require.Equal(t, 2, len(outer.fieldsByName))
	require.Contains(t, outer.fieldsByName, "id")
	require.Same(t, inner, outer.fieldsByName["nested"].message)

	require.Equal(t, 2, len(inner.fieldsByName))
	require.Contains(t, inner.fieldsByName, "id")
	require.Same(t, outer, inner.fieldsByName["nested"].message)
}

func TestCompileNestedMessages(t *testing.T) {
	const protoSchemaWithNestedMessages = `
	syntax = "proto3";
	package product.v1;

	option go_package = "grpc-graphql/pkg/proto/product/v1;productv1";

	message MyMessage {
		message NestedMessage {
			string nested_data = 1;
		}

		int32 first = 1;
		NestedMessage second = 2;
	}

	message SecondMessage {
	    message MyMessage {
		  message NestedMessage {
			string nested_data = 1;
		  }

		  NestedMessage nested_message = 1;
		}

		string second_data = 1;
		MyMessage third = 2;
	}
	`

	compiler, err := NewProtoCompiler(protoSchemaWithNestedMessages, nil)
	require.NoError(t, err)

	require.Equal(t, 5, len(compiler.doc.Messages))
	require.Equal(t, "MyMessage", compiler.doc.Messages[0].Name)
	require.Equal(t, "MyMessage.NestedMessage", compiler.doc.Messages[1].Name)
	require.Equal(t, "SecondMessage", compiler.doc.Messages[2].Name)
	require.Equal(t, "SecondMessage.MyMessage", compiler.doc.Messages[3].Name)
	require.Equal(t, "SecondMessage.MyMessage.NestedMessage", compiler.doc.Messages[4].Name)
}
