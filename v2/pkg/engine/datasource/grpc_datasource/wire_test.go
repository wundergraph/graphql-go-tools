package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/require"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// This file holds the schema and the helpers for wire_proto_test.go and wire_connect_test.go.

var testWireSchema = `
syntax = "proto3";
package test.wire.v1;

import "google/protobuf/wrappers.proto";

enum Status {
  STATUS_UNSPECIFIED = 0;
  STATUS_ACTIVE = 1;
  STATUS_INACTIVE = 2;
  STATUS_DELETED = -1;
}

message EmptyRequest {}

message ScalarRequest {
  string string_value = 1;
  int32 int_32 = 2;
  double double_value = 3;
  bool bool_value = 4;
  float float_value = 5;
  repeated float float_values = 6;

  // The GraphQL spec has no types for these. We support them, but they
  // need a custom scalar mapping.
  uint32 uint_32 = 7;
  uint64 uint_64 = 8;
  int64 int_64 = 9;

  // We do not support zigzag-encoded (sint) integers. Keep these for future reference.
  // sint32 sint_32 = 10;
  // sint64 sint_64 = 11;
}

message OptionalScalarRequest {
  optional string string_value = 1;
  optional float float_value = 2;
  optional double double_value = 3;
  optional bool bool_value = 4;
  optional uint32 uint_32_value = 5;
  optional uint64 uint_64_value = 6;
  optional int32 int_32_value = 7;
  optional int64 int_64_value = 8;
}

message WrapperScalarRequest {
  google.protobuf.StringValue string_value = 1;
  google.protobuf.Int32Value int_32 = 2;
  google.protobuf.DoubleValue double_value = 3;
  google.protobuf.BoolValue bool_value = 4;
}

message RepeatedScalarRequest {
  repeated string string_values = 1;
  repeated int32 int_32_values = 2;
}

message NestedItem {
  string id = 1;
  string value = 2;
}

message NestedMessageRequest {
  NestedItem item = 1;
  repeated NestedItem items = 2;
}

message ListOfString {
  message List {
    repeated string items = 1;
  }
  List list = 1;
}

message ListOfNestedItem {
  message List {
    repeated NestedItem items = 1;
  }
  List list = 1;
}

message ListOfListOfString {
  message List {
    repeated ListOfString items = 1;
  }
  List list = 1;
}

message ListOfListOfNestedItem {
  message List {
    repeated ListOfNestedItem items = 1;
  }
  List list = 1;
}

message ListWrapperRequest {
  ListOfString optional_tags = 1;
  ListOfNestedItem optional_items = 2;
}

message NestedListRequest {
  ListOfListOfString tag_groups = 1;
  ListOfListOfNestedItem item_groups = 2;
}

message EnumRequest {
  Status status = 1;
  repeated Status statuses = 2;
}

message ListOfStatus {
	message List {
		repeated Status items = 1;
	}
	List list = 1;
}


message ListOfListOfStatus {
	message List {
		repeated ListOfStatus items = 1;
	}
	List list = 1;
}

message EnumListRequest {
	ListOfStatus statuses = 1;
	ListOfListOfStatus status_groups = 2;
}

message MixedRequest {
  string id = 1;
  google.protobuf.StringValue description = 2;
  repeated string tags = 3;
  ListOfString keywords = 4;
  NestedItem metadata = 5;
  double price = 6;
  Status status = 7;
}

message LookupProductByIdRequestKey {
  string id = 1;
}

message LookupProductByIdRequest {
  repeated LookupProductByIdRequestKey keys = 1;
}

message InterfaceA {
	string name = 1;
}

message InterfaceB {
	string name = 1;
}

message UnionA {
	string name = 1;
}

message UnionB {
	string name = 1;
}

message InterfaceOneOfRequest {
	oneof instance {
		InterfaceA interface_a = 1;
		InterfaceB interface_b = 2;
	}
}

message UnionOneOfRequest {
	oneof value {
		UnionA value_a = 1;
		UnionB value_b = 2;
	}
}

message MetricContext {
  string id = 1;
  string metric_type = 2;
}

message MetricArgs {
  double baseline = 1;
}

message ResolveMetricRequest {
  repeated MetricContext context = 1;
  MetricArgs field_args = 2;
}

message Metric {
  string id = 1;
  string metric_type = 2;
}

message MetricsResponse {
  repeated Metric metrics = 1;
}

service TestService {
  rpc Empty(EmptyRequest) returns (EmptyRequest) {}
  rpc Scalar(ScalarRequest) returns (ScalarRequest) {}
  rpc WrapperScalar(WrapperScalarRequest) returns (WrapperScalarRequest) {}
  rpc RepeatedScalar(RepeatedScalarRequest) returns (RepeatedScalarRequest) {}
  rpc NestedMessage(NestedMessageRequest) returns (NestedMessageRequest) {}
  rpc ListWrapper(ListWrapperRequest) returns (ListWrapperRequest) {}
  rpc NestedList(NestedListRequest) returns (NestedListRequest) {}
  rpc Enum(EnumRequest) returns (EnumRequest) {}
  rpc Mixed(MixedRequest) returns (MixedRequest) {}
  rpc LookupProductById(LookupProductByIdRequest) returns (LookupProductByIdRequest) {}
}
`

func testWireMapping() *GRPCMapping {
	return &GRPCMapping{
		Service: "TestService",
		EnumValues: map[string][]EnumValueMapping{
			"Status": {
				{Value: "UNSPECIFIED", TargetValue: "STATUS_UNSPECIFIED"},
				{Value: "ACTIVE", TargetValue: "STATUS_ACTIVE"},
				{Value: "INACTIVE", TargetValue: "STATUS_INACTIVE"},
				{Value: "DELETED", TargetValue: "STATUS_DELETED"},
			},
		},
	}
}

func newWireTestRuntime(t *testing.T) *runtimeSchema {
	t.Helper()
	compiler, err := NewProtoCompiler(testWireSchema, testWireMapping())
	require.NoError(t, err)
	runtime, err := newSchemaRuntime(compiler.doc)
	require.NoError(t, err)
	return runtime
}

func compileTestWireMessage(t *testing.T, runtime *runtimeSchema, message *programMessage) *wireMessage {
	t.Helper()
	wm, err := compileWireMessage(runtime, message, make(map[string]*wireMessage))
	require.NoError(t, err)
	return wm
}

func compileTestProgramMessage(t *testing.T, runtime *runtimeSchema, rpcMessage *RPCMessage, rtMessage *runtimeMessage) *programMessage {
	t.Helper()

	message, err := compileMessage(runtime, rpcMessage, rtMessage, make(map[string]*programMessage))
	require.NoError(t, err)

	return message
}

// compileWrapperScalarRequest compiles WrapperScalarRequest with all of its
// wrapper fields as nullable scalars.
func compileWrapperScalarRequest(t *testing.T, runtime *runtimeSchema) *wireMessage {
	t.Helper()
	message := compileTestProgramMessage(t, runtime, &RPCMessage{
		Name: "WrapperScalarRequest",
		Fields: RPCFields{
			{Name: "string_value", ProtoTypeName: DataTypeString, JSONPath: "string_value", Optional: true},
			{Name: "int_32", ProtoTypeName: DataTypeInt32, JSONPath: "int_32", Optional: true},
			{Name: "double_value", ProtoTypeName: DataTypeDouble, JSONPath: "double_value", Optional: true},
			{Name: "bool_value", ProtoTypeName: DataTypeBool, JSONPath: "bool_value", Optional: true},
		},
	}, runtime.getMessageByName("WrapperScalarRequest"))
	return compileTestWireMessage(t, runtime, message)
}

// compileOptionalScalarRequest compiles OptionalScalarRequest with all of its
// proto3 optional fields as nullable scalars.
func compileOptionalScalarRequest(t *testing.T, runtime *runtimeSchema) *wireMessage {
	t.Helper()
	message := compileTestProgramMessage(t, runtime, optionalScalarRequestMessage(), runtime.getMessageByName("OptionalScalarRequest"))
	return compileTestWireMessage(t, runtime, message)
}

// optionalScalarRequestMessage returns OptionalScalarRequest with all of its
// proto3 optional fields as nullable scalars.
func optionalScalarRequestMessage() *RPCMessage {
	return &RPCMessage{
		Name: "OptionalScalarRequest",
		Fields: RPCFields{
			{Name: "string_value", ProtoTypeName: DataTypeString, JSONPath: "string_value", Optional: true},
			{Name: "double_value", ProtoTypeName: DataTypeDouble, JSONPath: "double_value", Optional: true},
			{Name: "float_value", ProtoTypeName: DataTypeFloat, JSONPath: "float_value", Optional: true},
			{Name: "bool_value", ProtoTypeName: DataTypeBool, JSONPath: "bool_value", Optional: true},
			{Name: "uint_32_value", ProtoTypeName: DataTypeUint32, JSONPath: "uint_32_value", Optional: true},
			{Name: "uint_64_value", ProtoTypeName: DataTypeUint64, JSONPath: "uint_64_value", Optional: true},
			{Name: "int_32_value", ProtoTypeName: DataTypeInt32, JSONPath: "int_32_value", Optional: true},
			{Name: "int_64_value", ProtoTypeName: DataTypeInt64, JSONPath: "int_64_value", Optional: true},
		},
	}
}

// Inputs for WrapperScalarRequest and OptionalScalarRequest. They set every
// field, either to null or to a value.
const (
	wrapperScalarNullInput = `{
		"string_value": null,
		"int_32": null,
		"double_value": null,
		"bool_value": null
	}`

	wrapperScalarValuesInput = `{
		"string_value": "hello",
		"int_32": 25,
		"double_value": 3.14,
		"bool_value": true
	}`

	optionalScalarNullInput = `{
		"string_value": null,
		"double_value": null,
		"float_value": null,
		"bool_value": null,
		"uint_32_value": null,
		"uint_64_value": null,
		"int_32_value": null,
		"int_64_value": null
	}`

	optionalScalarValuesInput = `{
		"string_value": "hello",
		"double_value": 3.14,
		"float_value": 1.5,
		"bool_value": true,
		"uint_32_value": 7,
		"uint_64_value": 8,
		"int_32_value": -9,
		"int_64_value": -10
	}`
)

// setWrapperValue sets a google.protobuf wrapper field (e.g. StringValue, Int32Value) on a dynamic message.
func setWrapperValue(msg *dynamicpb.Message, fieldName protoref.Name, value protoref.Value) {
	fd := msg.Descriptor().Fields().ByName(fieldName)
	wrapper := dynamicpb.NewMessage(fd.Message())
	wrapper.Set(fd.Message().Fields().ByName("value"), value)
	msg.Set(fd, protoref.ValueOfMessage(wrapper))
}
