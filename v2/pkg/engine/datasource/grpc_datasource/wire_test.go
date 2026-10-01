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
}

message EmptyRequest {}

message ScalarRequest {
  string name = 1;
  int32 age = 2;
  double score = 3;
  bool active = 4;
}

message WrapperScalarRequest {
  google.protobuf.StringValue name = 1;
  google.protobuf.Int32Value age = 2;
  google.protobuf.DoubleValue score = 3;
  google.protobuf.BoolValue active = 4;
}

message RepeatedScalarRequest {
  repeated string tags = 1;
  repeated int32 scores = 2;
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

// setWrapperValue sets a google.protobuf wrapper field (e.g. StringValue, Int32Value) on a dynamic message.
func setWrapperValue(msg *dynamicpb.Message, fieldName protoref.Name, value protoref.Value) {
	fd := msg.Descriptor().Fields().ByName(fieldName)
	wrapper := dynamicpb.NewMessage(fd.Message())
	wrapper.Set(fd.Message().Fields().ByName("value"), value)
	msg.Set(fd, protoref.ValueOfMessage(wrapper))
}
