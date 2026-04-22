package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewSchemaRuntime(t *testing.T) {
	compiler, err := NewProtoCompiler(testSchemaWithLookup, testMapping())
	require.NoError(t, err)

	runtime, err := newSchemaRuntime(compiler)
	require.NoError(t, err)

	require.Equal(t, 5, len(runtime.messageByName))
	require.Equal(t, 5, len(runtime.messageByFullname))
}

// =============== Test Schemas ================== //

var testSchemaWithLookup = `
syntax = "proto3";
package product.v1;

service ProductService {
  rpc LookupProductById(LookupProductByIdRequest) returns (LookupProductByIdResponse) {}
}

message LookupProductByIdRequest {
  repeated LookupProductByIdInput inputs = 1;
}

message LookupProductByIdInput {
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
