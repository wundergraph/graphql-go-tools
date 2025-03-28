package grpcdatasource

import (
	"testing"

	"github.com/tidwall/gjson"
)

var baseProto = `
syntax = "proto3";
package product.v1;
`

var validProto = baseProto + `

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

func TestCompile(t *testing.T) {
	compiler := NewProtoCompiler(validProto)
	err := compiler.Parse()
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}
}

func TestBuildProtoMessage(t *testing.T) {
	compiler := NewProtoCompiler(validProto)
	err := compiler.Parse()
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}

	executionPlan := &RPCExecutionPlan{
		Calls: []RPCCall{
			{
				ServiceName: "ProductService",
				MethodName:  "LookupProductById",
				Request: RPCMessage{
					Name: "LookupProductByIdRequest",
					Fields: []RPCField{
						{
							Name:     "inputs",
							TypeName: string(DataTypeMessage),
							Repeated: true,
							JSONPath: "variables.representations",
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
													JSONPath: "id",
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

	variables := []byte(`{"variables":{"representations":[{"__typename":"Product","id":"123"}]}}`)

	err = compiler.Compile(executionPlan, gjson.ParseBytes(variables))
	if err != nil {
		t.Fatalf("failed to compile proto: %v", err)
	}
}
