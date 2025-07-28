package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"google.golang.org/protobuf/reflect/protoreflect"
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

	plan := &RPCExecutionPlan{
		Calls: []RPCCall{
			{
				ServiceName: "Products",
				MethodName:  "QueryCalculateTotals",
				Request: RPCMessage{
					Name: "QueryCalculateTotalsRequest",
					Fields: []RPCField{
						{
							Name:     "orders",
							TypeName: string(DataTypeMessage),
							JSONPath: "orders",
							Repeated: true,
							Message: &RPCMessage{
								Name: "OrderInput",
								Fields: []RPCField{
									{
										Name:     "order_id",
										TypeName: string(DataTypeString),
										JSONPath: "orderId",
									},
									{
										Name:     "customer_name",
										TypeName: string(DataTypeString),
										JSONPath: "customerName",
									},
									{
										Name:     "lines",
										TypeName: string(DataTypeMessage),
										JSONPath: "lines",
										Repeated: true,
										Message: &RPCMessage{
											Name: "OrderLineInput",
											Fields: []RPCField{
												{
													Name:     "product_id",
													TypeName: string(DataTypeString),
													JSONPath: "productId",
												},
												{
													Name:     "quantity",
													TypeName: string(DataTypeInt32),
													JSONPath: "quantity",
												},
												{
													Name:       "modifiers",
													TypeName:   string(DataTypeString),
													JSONPath:   "modifiers",
													Optional:   true,
													IsListType: true,
													ListMetadata: &ListMetadata{
														NestingLevel: 1,
														LevelInfo: []LevelInfo{
															{
																Optional: true,
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
				},
				Response: RPCMessage{
					Name: "QueryCalculateTotalsResponse",
					Fields: []RPCField{
						{
							Name:     "calculate_totals",
							TypeName: string(DataTypeMessage),
							JSONPath: "calculateTotals",
							Repeated: true,
							Message: &RPCMessage{
								Name: "Order",
								Fields: []RPCField{
									{
										Name:     "order_id",
										TypeName: string(DataTypeString),
										JSONPath: "orderId",
									},
									{
										Name:     "customer_name",
										TypeName: string(DataTypeString),
										JSONPath: "customerName",
									},
									{
										Name:     "total_items",
										TypeName: string(DataTypeInt32),
										JSONPath: "totalItems",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	invocations, err := compiler.Compile(plan, gjson.ParseBytes([]byte(`{"orders":[{"orderId":"123","customerName":"John Doe","lines":[{"productId":"123","quantity":1, "modifiers":["modifier1", "modifier2"]}]}]}`)))
	require.NoError(t, err)
	require.Equal(t, 1, len(invocations))

	proto := invocations[0].Input.ProtoReflect()

	msgDesc := proto.Descriptor()

	ordersDesc := msgDesc.Fields().ByName("orders")
	require.True(t, proto.Has(ordersDesc))
	orders := proto.Get(ordersDesc)
	require.True(t, orders.IsValid())

	require.True(t, ordersDesc.IsList())
	ordersList := orders.List()

	orderMsg := ordersList.Get(0).Message()
	orderDesc := orderMsg.Descriptor()
	require.True(t, ordersList.Get(0).Message().Has(orderDesc.Fields().ByName("order_id")))
	require.True(t, ordersList.Get(0).Message().Has(orderDesc.Fields().ByName("customer_name")))
	require.True(t, ordersList.Get(0).Message().Has(orderDesc.Fields().ByName("lines")))

	linesList := ordersList.Get(0).Message().Get(orderDesc.Fields().ByName("lines")).List()
	require.True(t, linesList.IsValid())

	require.Equal(t, 1, linesList.Len())

	for i := 0; i < linesList.Len(); i++ {
		linesMsg := linesList.Get(i).Message()

		linesDesc := linesMsg.Descriptor().Fields()

		require.True(t, linesMsg.Has(linesDesc.ByName("product_id")))
		require.True(t, linesMsg.Has(linesDesc.ByName("quantity")))
		require.True(t, linesMsg.Has(linesDesc.ByName("modifiers")))

		modifiersDesc := linesDesc.ByName("modifiers")
		require.True(t, modifiersDesc.Kind() == protoreflect.MessageKind)

		modifiersMsg := linesMsg.Get(modifiersDesc).Message()
		require.True(t, modifiersMsg.IsValid(), "expected modifiers message to be valid")
		modifiersListMsg := modifiersMsg.Get(modifiersMsg.Descriptor().Fields().ByName("list")).Message()
		modifiersList := modifiersListMsg.Get(modifiersListMsg.Descriptor().Fields().ByName("items")).List()

		require.True(t, modifiersList.IsValid(), "expected modifiers list to be valid")
		require.Equal(t, 2, modifiersList.Len())
		require.Equal(t, "modifier1", modifiersList.Get(0).String())
		require.Equal(t, "modifier2", modifiersList.Get(1).String())
	}
}
