package grpcdatasource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/astjson"
)

// marshalDynamic builds a dynamicpb message using the runtime's descriptor and marshals it via proto.Marshal.
// This produces the canonical protobuf encoding to compare against createProtoWire output.
func marshalDynamic(t *testing.T, runtime *runtimeSchema, messageName string, build func(msg *dynamicpb.Message, desc protoref.MessageDescriptor)) []byte {
	t.Helper()
	rtMsg := runtime.getMessageByName(messageName)
	require.NotNilf(t, rtMsg, "message %q not found in runtime", messageName)
	msg := dynamicpb.NewMessage(rtMsg.desc)
	build(msg, rtMsg.desc)
	out, err := proto.Marshal(msg)
	require.NoError(t, err)
	return out
}

// assertProtoEqual unmarshals both byte slices into the same message type and compares the resulting messages.
// This allows valid encoding differences (e.g. packed vs unpacked repeated scalars) to pass.
func assertProtoEqual(t *testing.T, runtime *runtimeSchema, messageName string, expectedMessageBytes, gotMessageBytes []byte) {
	t.Helper()
	rtMsg := runtime.getMessageByName(messageName)
	require.NotNilf(t, rtMsg, "message %q not found in runtime", messageName)

	expectedMsg := dynamicpb.NewMessage(rtMsg.desc)
	require.NoError(t, proto.Unmarshal(expectedMessageBytes, expectedMsg), "failed to unmarshal expected bytes")

	gotMsg := dynamicpb.NewMessage(rtMsg.desc)
	require.NoError(t, proto.Unmarshal(gotMessageBytes, gotMsg), "failed to unmarshal got bytes %x", gotMessageBytes)

	assert.True(t, proto.Equal(expectedMsg, gotMsg),
		"messages not equal\nexpected: %v\ngot:      %v\nexpected bytes: %x\ngot bytes:      %x",
		expectedMsg, gotMsg, expectedMessageBytes, gotMessageBytes)
}

func TestCompileWireMessage(t *testing.T) {
	runtime := newWireTestRuntime(t)

	t.Run("empty message with no fields", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name:   "EmptyRequest",
			Fields: nil,
		}, runtime.getMessageByName("EmptyRequest"))

		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 0)
	})

	t.Run("scalar fields", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				{Name: "age", ProtoTypeName: DataTypeInt32, JSONPath: "age"},
				{Name: "score", ProtoTypeName: DataTypeDouble, JSONPath: "score"},
				{Name: "active", ProtoTypeName: DataTypeBool, JSONPath: "active"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 4)
		assert.Equal(t, DataTypeString, wm.fields[0].dataType)
		assert.Equal(t, DataTypeInt32, wm.fields[1].dataType)
		assert.Equal(t, DataTypeDouble, wm.fields[2].dataType)
		assert.Equal(t, DataTypeBool, wm.fields[3].dataType)
	})

	t.Run("wrapper scalar fields as optional scalars", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true},
				{Name: "age", ProtoTypeName: DataTypeInt32, JSONPath: "age", Optional: true},
				{Name: "score", ProtoTypeName: DataTypeDouble, JSONPath: "score", Optional: true},
				{Name: "active", ProtoTypeName: DataTypeBool, JSONPath: "active", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 4)
		assert.True(t, wm.fields[0].optional)
		assert.True(t, wm.fields[1].optional)
		assert.True(t, wm.fields[2].optional)
		assert.True(t, wm.fields[3].optional)
	})

	t.Run("repeated scalar fields", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "RepeatedScalarRequest",
			Fields: RPCFields{
				{Name: "tags", ProtoTypeName: DataTypeString, JSONPath: "tags", Repeated: true},
				{Name: "scores", ProtoTypeName: DataTypeInt32, JSONPath: "scores", Repeated: true},
			},
		}, runtime.getMessageByName("RepeatedScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 2)
		assert.True(t, wm.fields[0].repeated)
		assert.True(t, wm.fields[1].repeated)
	})

	t.Run("list wrapper with list metadata", func(t *testing.T) {
		msg := runtime.getMessageByName("ListWrapperRequest")
		require.NotNil(t, msg)

		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ListWrapperRequest",
			Fields: RPCFields{
				{
					Name:          "optional_tags",
					ProtoTypeName: DataTypeString,
					JSONPath:      "optionalTags",
					Optional:      true,
					IsListType:    true,
					ListMetadata: &ListMetadata{
						NestingLevel: 1,
						LevelInfo:    []LevelInfo{{Optional: true}},
					},
				},
			},
		}, msg)

		// Optional + IsListType: compileWireMessage must not treat this as a wrapper scalar.
		// Currently this errors because it tries to wrap in google.protobuf.*Value and looks for "value" in ListOfString.
		wm, err := compileWireMessage(runtime, message, make(map[string]*wireMessage))

		require.NoError(t, err)

		assert.Len(t, wm.fields, 1)
		assert.NotNil(t, wm.fields[0].listMetadata)
		assert.Equal(t, 1, wm.fields[0].listMetadata.NestingLevel)
	})

	t.Run("nested list wrapper with list metadata", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "NestedListRequest",
			Fields: RPCFields{
				{
					Name:          "tag_groups",
					ProtoTypeName: DataTypeString,
					JSONPath:      "tagGroups",
					IsListType:    true,
					ListMetadata: &ListMetadata{
						NestingLevel: 2,
						LevelInfo:    []LevelInfo{{Optional: false}, {Optional: false}},
					},
				},
			},
		}, runtime.getMessageByName("NestedListRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 1)
		assert.NotNil(t, wm.fields[0].listMetadata)
		assert.Equal(t, 2, wm.fields[0].listMetadata.NestingLevel)
	})

	t.Run("enum field", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumRequest",
			Fields: RPCFields{
				{Name: "status", ProtoTypeName: DataTypeEnum, JSONPath: "status", EnumName: "Status"},
				{Name: "statuses", ProtoTypeName: DataTypeEnum, JSONPath: "statuses", EnumName: "Status", Repeated: true},
			},
		}, runtime.getMessageByName("EnumRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 2)
		assert.Equal(t, DataTypeEnum, wm.fields[0].dataType)
		assert.Equal(t, DataTypeEnum, wm.fields[1].dataType)
		assert.True(t, wm.fields[1].repeated)
	})

	t.Run("entity lookup request", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "LookupProductByIdRequest",
			Fields: RPCFields{
				{
					Name:          "keys",
					ProtoTypeName: DataTypeMessage,
					Repeated:      true,
					JSONPath:      "representations",
					Message: &RPCMessage{
						Name:        "LookupProductByIdRequestKey",
						MemberTypes: []string{"Product"},
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
						},
					},
				},
			},
		}, runtime.getMessageByName("LookupProductByIdRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		assert.Len(t, wm.fields, 1)
		assert.True(t, wm.fields[0].repeated)
		assert.Equal(t, DataTypeMessage, wm.fields[0].dataType)
		assert.NotNil(t, wm.fields[0].child)
		assert.Len(t, wm.fields[0].child.fields, 1)
	})
}

func TestCreateProtoWire(t *testing.T) {
	runtime := newWireTestRuntime(t)

	t.Run("empty message", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name:   "EmptyRequest",
			Fields: nil,
		}, runtime.getMessageByName("EmptyRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "EmptyRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {})

		assertProtoEqual(t, runtime, "EmptyRequest", expected, got)
	})

	t.Run("single string field", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"name":"hello"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "ScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("name"), protoref.ValueOfString("hello"))
		})

		assertProtoEqual(t, runtime, "ScalarRequest", expected, got)
	})

	t.Run("string int32 and double fields", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				{Name: "age", ProtoTypeName: DataTypeInt32, JSONPath: "age"},
				{Name: "score", ProtoTypeName: DataTypeDouble, JSONPath: "score"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"name":"alice","age":30,"score":99.5}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "ScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("name"), protoref.ValueOfString("alice"))
			msg.Set(desc.Fields().ByName("age"), protoref.ValueOfInt32(30))
			msg.Set(desc.Fields().ByName("score"), protoref.ValueOfFloat64(99.5))
		})

		assertProtoEqual(t, runtime, "ScalarRequest", expected, got)
	})

	t.Run("int32 and double fields with negative values", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "age", ProtoTypeName: DataTypeInt32, JSONPath: "age"},
				{Name: "score", ProtoTypeName: DataTypeDouble, JSONPath: "score"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		got, err := wm.createProtoWire(astjson.MustParse(`{"age":-7,"score":-99.5}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "ScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("age"), protoref.ValueOfInt32(-7))
			msg.Set(desc.Fields().ByName("score"), protoref.ValueOfFloat64(-99.5))
		})

		assertProtoEqual(t, runtime, "ScalarRequest", expected, got)
	})

	t.Run("wrapper string value present", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{
					Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true,
				},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"name":"hello"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			setWrapperValue(msg, "name", protoref.ValueOfString("hello"))
		})

		assertProtoEqual(t, runtime, "WrapperScalarRequest", expected, got)
	})

	t.Run("wrapper string value absent", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			// name not set — wrapper absent means null
		})

		assertProtoEqual(t, runtime, "WrapperScalarRequest", expected, got)
	})

	t.Run("wrapper string value explicit null", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"name":null}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			// explicit null means unset, no Set call.
		})

		assertProtoEqual(t, runtime, "WrapperScalarRequest", expected, got)
	})

	t.Run("wrapper int32 and double values", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "age", ProtoTypeName: DataTypeInt32, JSONPath: "age", Optional: true},
				{Name: "score", ProtoTypeName: DataTypeDouble, JSONPath: "score", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"age":25,"score":3.14}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			setWrapperValue(msg, "age", protoref.ValueOfInt32(25))
			setWrapperValue(msg, "score", protoref.ValueOfFloat64(3.14))
		})

		assertProtoEqual(t, runtime, "WrapperScalarRequest", expected, got)
	})

	t.Run("repeated strings", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "RepeatedScalarRequest",
			Fields: RPCFields{
				{Name: "tags", ProtoTypeName: DataTypeString, JSONPath: "tags", Repeated: true},
			},
		}, runtime.getMessageByName("RepeatedScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"tags":["foo","bar","baz"]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "RepeatedScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("tags")).List()
			list.Append(protoref.ValueOfString("foo"))
			list.Append(protoref.ValueOfString("bar"))
			list.Append(protoref.ValueOfString("baz"))
		})

		assertProtoEqual(t, runtime, "RepeatedScalarRequest", expected, got)
	})

	t.Run("repeated int32s", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "RepeatedScalarRequest",
			Fields: RPCFields{
				{Name: "scores", ProtoTypeName: DataTypeInt32, JSONPath: "scores", Repeated: true},
			},
		}, runtime.getMessageByName("RepeatedScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"scores":[1,2,3]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "RepeatedScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("scores")).List()
			list.Append(protoref.ValueOfInt32(1))
			list.Append(protoref.ValueOfInt32(2))
			list.Append(protoref.ValueOfInt32(3))
		})

		assertProtoEqual(t, runtime, "RepeatedScalarRequest", expected, got)
	})

	t.Run("single nested message", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "NestedMessageRequest",
			Fields: RPCFields{
				{
					Name: "item", ProtoTypeName: DataTypeMessage, JSONPath: "item",
					Message: &RPCMessage{
						Name: "NestedItem",
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
							{Name: "value", ProtoTypeName: DataTypeString, JSONPath: "value"},
						},
					},
				},
			},
		}, runtime.getMessageByName("NestedMessageRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"item":{"id":"1","value":"a"}}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "NestedMessageRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			itemField := desc.Fields().ByName("item")
			item := dynamicpb.NewMessage(itemField.Message())
			item.Set(itemField.Message().Fields().ByName("id"), protoref.ValueOfString("1"))
			item.Set(itemField.Message().Fields().ByName("value"), protoref.ValueOfString("a"))
			msg.Set(itemField, protoref.ValueOfMessage(item))
		})

		assertProtoEqual(t, runtime, "NestedMessageRequest", expected, got)
	})

	t.Run("repeated nested messages", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "NestedMessageRequest",
			Fields: RPCFields{
				{
					Name: "items", ProtoTypeName: DataTypeMessage, JSONPath: "items", Repeated: true,
					Message: &RPCMessage{
						Name: "NestedItem",
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
							{Name: "value", ProtoTypeName: DataTypeString, JSONPath: "value"},
						},
					},
				},
			},
		}, runtime.getMessageByName("NestedMessageRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"items":[{"id":"1","value":"a"},{"id":"2","value":"b"}]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "NestedMessageRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			itemsField := desc.Fields().ByName("items")
			itemDesc := itemsField.Message()
			list := msg.Mutable(itemsField).List()

			item1 := dynamicpb.NewMessage(itemDesc)
			item1.Set(itemDesc.Fields().ByName("id"), protoref.ValueOfString("1"))
			item1.Set(itemDesc.Fields().ByName("value"), protoref.ValueOfString("a"))
			list.Append(protoref.ValueOfMessage(item1))

			item2 := dynamicpb.NewMessage(itemDesc)
			item2.Set(itemDesc.Fields().ByName("id"), protoref.ValueOfString("2"))
			item2.Set(itemDesc.Fields().ByName("value"), protoref.ValueOfString("b"))
			list.Append(protoref.ValueOfMessage(item2))
		})

		assertProtoEqual(t, runtime, "NestedMessageRequest", expected, got)
	})

	t.Run("enum field", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumRequest",
			Fields: RPCFields{
				{Name: "status", ProtoTypeName: DataTypeEnum, JSONPath: "status", EnumName: "Status"},
			},
		}, runtime.getMessageByName("EnumRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		// GraphQL sends enum values as strings (e.g. "ACTIVE"), not proto-prefixed names or integers.
		// The wire builder must resolve "ACTIVE" -> STATUS_ACTIVE = 1 via the runtime enum map.
		got, err := wm.createProtoWire(astjson.MustParse(`{"status":"ACTIVE"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "EnumRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("status"), protoref.ValueOfEnum(1))
		})

		assertProtoEqual(t, runtime, "EnumRequest", expected, got)
	})

	t.Run("repeated enums", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumRequest",
			Fields: RPCFields{
				{Name: "statuses", ProtoTypeName: DataTypeEnum, JSONPath: "statuses", EnumName: "Status", Repeated: true},
			},
		}, runtime.getMessageByName("EnumRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"statuses":["UNSPECIFIED","ACTIVE","INACTIVE"]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "EnumRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("statuses")).List()
			list.Append(protoref.ValueOfEnum(0))
			list.Append(protoref.ValueOfEnum(1))
			list.Append(protoref.ValueOfEnum(2))
		})

		assertProtoEqual(t, runtime, "EnumRequest", expected, got)
	})

	t.Run("list wrapper with strings", func(t *testing.T) {
		// RPC plan models ListOfString as a flat optional scalar with IsListType + ListMetadata.
		// createProtoWire must produce: ListWrapperRequest { optional_tags: ListOfString { list: List { items: [...] } } }
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ListWrapperRequest",
			Fields: RPCFields{
				{
					Name:          "optional_tags",
					ProtoTypeName: DataTypeString,
					JSONPath:      "optionalTags",
					Optional:      true,
					IsListType:    true,
					ListMetadata: &ListMetadata{
						NestingLevel: 1,
						LevelInfo:    []LevelInfo{{Optional: true}},
					},
				},
			},
		}, runtime.getMessageByName("ListWrapperRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"optionalTags":["a","b"]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "ListWrapperRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			// Build: ListWrapperRequest { optional_tags: ListOfString { list: List { items: ["a","b"] } } }
			optTagsField := desc.Fields().ByName("optional_tags")
			listOfStringDesc := optTagsField.Message()

			listField := listOfStringDesc.Fields().ByName("list")
			listDesc := listField.Message()

			innerList := dynamicpb.NewMessage(listDesc)
			items := innerList.Mutable(listDesc.Fields().ByName("items")).List()
			items.Append(protoref.ValueOfString("a"))
			items.Append(protoref.ValueOfString("b"))

			listOfString := dynamicpb.NewMessage(listOfStringDesc)
			listOfString.Set(listField, protoref.ValueOfMessage(innerList))

			msg.Set(optTagsField, protoref.ValueOfMessage(listOfString))
		})

		assertProtoEqual(t, runtime, "ListWrapperRequest", expected, got)
	})

	t.Run("list wrapper with enums", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumListRequest",
			Fields: RPCFields{
				{Name: "statuses", ProtoTypeName: DataTypeEnum, JSONPath: "statuses", IsListType: true, ListMetadata: &ListMetadata{NestingLevel: 1, LevelInfo: []LevelInfo{{Optional: false}}}, EnumName: "Status"},
			},
		}, runtime.getMessageByName("EnumListRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		got, err := wm.createProtoWire(astjson.MustParse(`{"statuses":["ACTIVE","INACTIVE"]}`))
		require.NoError(t, err)

		e := runtime.enumByName["Status"]
		activeValue := e.valuesByName["ACTIVE"]
		inactiveValue := e.valuesByName["INACTIVE"]

		expected := marshalDynamic(t, runtime, "EnumListRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			// Build: ListWrapperRequest { optional_tags: ListOfString { list: List { items: ["a","b"] } } }
			statusesField := desc.Fields().ByName("statuses")
			listOfStringDesc := statusesField.Message()

			listField := listOfStringDesc.Fields().ByName("list")
			listDesc := listField.Message()

			innerList := dynamicpb.NewMessage(listDesc)
			items := innerList.Mutable(listDesc.Fields().ByName("items")).List()
			items.Append(protoref.ValueOfEnum(protoref.EnumNumber(activeValue.value)))
			items.Append(protoref.ValueOfEnum(protoref.EnumNumber(inactiveValue.value)))

			listOfString := dynamicpb.NewMessage(listOfStringDesc)
			listOfString.Set(listField, protoref.ValueOfMessage(innerList))

			msg.Set(statusesField, protoref.ValueOfMessage(listOfString))
		})

		require.NoError(t, err)
		assertProtoEqual(t, runtime, "EnumListRequest", expected, got)
	})

	t.Run("nested list wrapper two levels", func(t *testing.T) {
		// RPC plan models ListOfListOfString as a flat scalar with IsListType + ListMetadata (NestingLevel=2).
		// createProtoWire must produce: NestedListRequest { tag_groups: ListOfListOfString { list: { items: [ ListOfString{...}, ... ] } } }
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "NestedListRequest",
			Fields: RPCFields{
				{
					Name:          "tag_groups",
					ProtoTypeName: DataTypeString,
					JSONPath:      "tagGroups",
					IsListType:    true,
					ListMetadata: &ListMetadata{
						NestingLevel: 2,
						LevelInfo:    []LevelInfo{{Optional: false}, {Optional: false}},
					},
				},
			},
		}, runtime.getMessageByName("NestedListRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"tagGroups":[["a","b"],["c"]]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "NestedListRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			tagGroupsField := desc.Fields().ByName("tag_groups")
			lolosDesc := tagGroupsField.Message() // ListOfListOfString

			outerListField := lolosDesc.Fields().ByName("list")
			outerListDesc := outerListField.Message() // ListOfListOfString.List
			outerItemsField := outerListDesc.Fields().ByName("items")
			losDesc := outerItemsField.Message() // ListOfString

			// Build ListOfString for ["a","b"]
			buildListOfString := func(values ...string) *dynamicpb.Message {
				innerListField := losDesc.Fields().ByName("list")
				innerListDesc := innerListField.Message()
				innerList := dynamicpb.NewMessage(innerListDesc)
				items := innerList.Mutable(innerListDesc.Fields().ByName("items")).List()
				for _, v := range values {
					items.Append(protoref.ValueOfString(v))
				}
				los := dynamicpb.NewMessage(losDesc)
				los.Set(innerListField, protoref.ValueOfMessage(innerList))
				return los
			}

			outerList := dynamicpb.NewMessage(outerListDesc)
			outerItems := outerList.Mutable(outerItemsField).List()
			outerItems.Append(protoref.ValueOfMessage(buildListOfString("a", "b")))
			outerItems.Append(protoref.ValueOfMessage(buildListOfString("c")))

			lolos := dynamicpb.NewMessage(lolosDesc)
			lolos.Set(outerListField, protoref.ValueOfMessage(outerList))

			msg.Set(tagGroupsField, protoref.ValueOfMessage(lolos))
		})

		assertProtoEqual(t, runtime, "NestedListRequest", expected, got)
	})

	t.Run("nested list wrapper two levels with messages", func(t *testing.T) {
		// NestedListRequest { item_groups: ListOfListOfNestedItem { list: { items: [ ListOfNestedItem{...}, ... ] } } }
		// The inner ListOfNestedItem contains NestedItem messages with id + value fields.
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "NestedListRequest",
			Fields: RPCFields{
				{
					Name:          "item_groups",
					ProtoTypeName: DataTypeMessage,
					JSONPath:      "itemGroups",
					IsListType:    true,
					ListMetadata: &ListMetadata{
						NestingLevel: 2,
						LevelInfo:    []LevelInfo{{Optional: false}, {Optional: false}},
					},
					Message: &RPCMessage{
						Name: "NestedItem",
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
							{Name: "value", ProtoTypeName: DataTypeString, JSONPath: "value"},
						},
					},
				},
			},
		}, runtime.getMessageByName("NestedListRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"itemGroups":[[{"id":"1","value":"a"},{"id":"2","value":"b"}],[{"id":"3","value":"c"}]]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "NestedListRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			itemGroupsField := desc.Fields().ByName("item_groups")
			loloniDesc := itemGroupsField.Message() // ListOfListOfNestedItem

			outerListField := loloniDesc.Fields().ByName("list")
			outerListDesc := outerListField.Message() // ListOfListOfNestedItem.List
			outerItemsField := outerListDesc.Fields().ByName("items")
			loniDesc := outerItemsField.Message() // ListOfNestedItem

			// Helper: build a ListOfNestedItem from NestedItem values
			buildListOfNestedItem := func(items ...struct{ id, value string }) *dynamicpb.Message {
				innerListField := loniDesc.Fields().ByName("list")
				innerListDesc := innerListField.Message()                          // ListOfNestedItem.List
				nestedItemDesc := innerListDesc.Fields().ByName("items").Message() // NestedItem

				innerList := dynamicpb.NewMessage(innerListDesc)
				itemsList := innerList.Mutable(innerListDesc.Fields().ByName("items")).List()
				for _, item := range items {
					ni := dynamicpb.NewMessage(nestedItemDesc)
					ni.Set(nestedItemDesc.Fields().ByName("id"), protoref.ValueOfString(item.id))
					ni.Set(nestedItemDesc.Fields().ByName("value"), protoref.ValueOfString(item.value))
					itemsList.Append(protoref.ValueOfMessage(ni))
				}
				loni := dynamicpb.NewMessage(loniDesc)
				loni.Set(innerListField, protoref.ValueOfMessage(innerList))
				return loni
			}

			outerList := dynamicpb.NewMessage(outerListDesc)
			outerItems := outerList.Mutable(outerItemsField).List()
			outerItems.Append(protoref.ValueOfMessage(buildListOfNestedItem(
				struct{ id, value string }{"1", "a"},
				struct{ id, value string }{"2", "b"},
			)))
			outerItems.Append(protoref.ValueOfMessage(buildListOfNestedItem(
				struct{ id, value string }{"3", "c"},
			)))

			lolosni := dynamicpb.NewMessage(loloniDesc)
			lolosni.Set(outerListField, protoref.ValueOfMessage(outerList))

			msg.Set(itemGroupsField, protoref.ValueOfMessage(lolosni))
		})

		assertProtoEqual(t, runtime, "NestedListRequest", expected, got)
	})

	t.Run("oneOf request with interface", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name:        "InterfaceOneOfRequest",
			MemberTypes: []string{"InterfaceA", "InterfaceB"},
			OneOfType:   OneOfTypeInterface,
			FragmentFields: RPCFieldSelectionSet{
				"InterfaceA": {
					{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				},
				"InterfaceB": {
					{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				},
			},
		}, runtime.getMessageByName("InterfaceOneOfRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		got, err := wm.createProtoWire(astjson.MustParse(`{"__typename":"InterfaceA","name":"A"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "InterfaceOneOfRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			oneOfDesc := desc.Oneofs().ByName("instance")
			require.NotNil(t, oneOfDesc)
			ifa := oneOfDesc.Fields().ByName("interface_a")
			require.NotNil(t, ifa)

			ifaMsg := dynamicpb.NewMessage(ifa.Message())
			ifaFieldDesc := ifa.Message().Fields()
			require.NotNil(t, ifaFieldDesc)

			ifaMsg.Set(ifaFieldDesc.ByName("name"), protoref.ValueOfString("A"))
			msg.Set(ifa, protoref.ValueOfMessage(ifaMsg))
		})
		assertProtoEqual(t, runtime, "InterfaceOneOfRequest", expected, got)

		got, err = wm.createProtoWire(astjson.MustParse(`{"__typename":"InterfaceB","name":"B"}`))
		require.NoError(t, err)

		expected = marshalDynamic(t, runtime, "InterfaceOneOfRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			oneOfDesc := desc.Oneofs().ByName("instance")
			require.NotNil(t, oneOfDesc)
			ib := oneOfDesc.Fields().ByName("interface_b")
			require.NotNil(t, ib)

			ibMsg := dynamicpb.NewMessage(ib.Message())
			ibFieldDesc := ib.Message().Fields()
			require.NotNil(t, ibFieldDesc)
			ibMsg.Set(ibFieldDesc.ByName("name"), protoref.ValueOfString("B"))
			msg.Set(ib, protoref.ValueOfMessage(ibMsg))
		})

		assertProtoEqual(t, runtime, "InterfaceOneOfRequest", expected, got)
	})

	t.Run("oneOf request with union", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name:        "UnionOneOfRequest",
			MemberTypes: []string{"UnionA", "UnionB"},
			OneOfType:   OneOfTypeUnion,
			FragmentFields: RPCFieldSelectionSet{
				"UnionA": {
					{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				},
				"UnionB": {
					{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
				},
			},
		}, runtime.getMessageByName("UnionOneOfRequest"))
		wm := compileTestWireMessage(t, runtime, message)
		got, err := wm.createProtoWire(astjson.MustParse(`{"__typename":"UnionA","name":"A"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "UnionOneOfRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			oneOfDesc := desc.Oneofs().ByName("value")
			require.NotNil(t, oneOfDesc)
			ua := oneOfDesc.Fields().ByName("value_a")
			require.NotNil(t, ua)

			uaMsg := dynamicpb.NewMessage(ua.Message())
			uaFieldDesc := ua.Message().Fields()
			require.NotNil(t, uaFieldDesc)
			uaMsg.Set(uaFieldDesc.ByName("name"), protoref.ValueOfString("A"))
			msg.Set(ua, protoref.ValueOfMessage(uaMsg))
		})

		assertProtoEqual(t, runtime, "UnionOneOfRequest", expected, got)

		got, err = wm.createProtoWire(astjson.MustParse(`{"__typename":"UnionB","name":"B"}`))
		require.NoError(t, err)

		expected = marshalDynamic(t, runtime, "UnionOneOfRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			oneOfDesc := desc.Oneofs().ByName("value")
			require.NotNil(t, oneOfDesc)
			ub := oneOfDesc.Fields().ByName("value_b")
			require.NotNil(t, ub)

			ubMsg := dynamicpb.NewMessage(ub.Message())
			ubFieldDesc := ub.Message().Fields()
			require.NotNil(t, ubFieldDesc)
			ubMsg.Set(ubFieldDesc.ByName("name"), protoref.ValueOfString("B"))
			msg.Set(ub, protoref.ValueOfMessage(ubMsg))
		})

		assertProtoEqual(t, runtime, "UnionOneOfRequest", expected, got)
	})

	t.Run("mixed request with multiple field types", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "MixedRequest",
			Fields: RPCFields{
				{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
				{Name: "description", ProtoTypeName: DataTypeString, JSONPath: "description", Optional: true},
				{Name: "tags", ProtoTypeName: DataTypeString, JSONPath: "tags", Repeated: true},
				{Name: "price", ProtoTypeName: DataTypeDouble, JSONPath: "price"},
				{Name: "status", ProtoTypeName: DataTypeEnum, JSONPath: "status", EnumName: "Status"},
			},
		}, runtime.getMessageByName("MixedRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"id":"p1","description":"a product","tags":["sale","new"],"price":29.99,"status":"ACTIVE"}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "MixedRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("id"), protoref.ValueOfString("p1"))
			setWrapperValue(msg, "description", protoref.ValueOfString("a product"))
			tagsList := msg.Mutable(desc.Fields().ByName("tags")).List()
			tagsList.Append(protoref.ValueOfString("sale"))
			tagsList.Append(protoref.ValueOfString("new"))
			msg.Set(desc.Fields().ByName("price"), protoref.ValueOfFloat64(29.99))
			msg.Set(desc.Fields().ByName("status"), protoref.ValueOfEnum(1))
		})

		assertProtoEqual(t, runtime, "MixedRequest", expected, got)
	})

	t.Run("entity lookup single key", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "LookupProductByIdRequest",
			Fields: RPCFields{
				{
					Name:          "keys",
					ProtoTypeName: DataTypeMessage,
					Repeated:      true,
					JSONPath:      "representations",
					Message: &RPCMessage{
						Name:        "LookupProductByIdRequestKey",
						MemberTypes: []string{"Product"},
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
						},
					},
				},
			},
		}, runtime.getMessageByName("LookupProductByIdRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"representations":[{"__typename":"Product","id":"1"}]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "LookupProductByIdRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			keysField := desc.Fields().ByName("keys")
			keyDesc := keysField.Message()
			list := msg.Mutable(keysField).List()

			key1 := dynamicpb.NewMessage(keyDesc)
			key1.Set(keyDesc.Fields().ByName("id"), protoref.ValueOfString("1"))
			list.Append(protoref.ValueOfMessage(key1))
		})

		assertProtoEqual(t, runtime, "LookupProductByIdRequest", expected, got)
	})

	t.Run("entity lookup multiple keys", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "LookupProductByIdRequest",
			Fields: RPCFields{
				{
					Name:          "keys",
					ProtoTypeName: DataTypeMessage,
					Repeated:      true,
					JSONPath:      "representations",
					Message: &RPCMessage{
						Name:        "LookupProductByIdRequestKey",
						MemberTypes: []string{"Product"},
						Fields: RPCFields{
							{Name: "id", ProtoTypeName: DataTypeString, JSONPath: "id"},
						},
					},
				},
			},
		}, runtime.getMessageByName("LookupProductByIdRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoWire(astjson.MustParse(`{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"},{"__typename":"Product","id":"3"}]}`))
		require.NoError(t, err)

		expected := marshalDynamic(t, runtime, "LookupProductByIdRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			keysField := desc.Fields().ByName("keys")
			keyDesc := keysField.Message()
			list := msg.Mutable(keysField).List()

			for _, id := range []string{"1", "2", "3"} {
				key := dynamicpb.NewMessage(keyDesc)
				key.Set(keyDesc.Fields().ByName("id"), protoref.ValueOfString(id))
				list.Append(protoref.ValueOfMessage(key))
			}
		})

		assertProtoEqual(t, runtime, "LookupProductByIdRequest", expected, got)
	})
}
