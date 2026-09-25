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

// buildExpectedProto creates a dynamicpb message of the given type and lets the
// caller populate it. The returned message is the canonical proto representation
// used to compare against createProtoMessage output via proto.Equal.
func buildExpectedProto(t *testing.T, runtime *runtimeSchema, messageName string, build func(msg *dynamicpb.Message, desc protoref.MessageDescriptor)) *dynamicpb.Message {
	t.Helper()
	rtMsg := runtime.getMessageByName(messageName)
	require.NotNilf(t, rtMsg, "message %q not found in runtime", messageName)
	msg := dynamicpb.NewMessage(rtMsg.desc)
	build(msg, rtMsg.desc)
	return msg
}

// assertProtoMessageEqual compares two proto messages, failing with a readable
// diff if they differ. The comparison is value-based: dynamicpb messages of the
// same descriptor with the same field values are equal regardless of how they
// were constructed.
func assertProtoMessageEqual(t *testing.T, expected, got protoref.Message) {
	t.Helper()
	assert.True(t, proto.Equal(expected.Interface(), got.Interface()),
		"messages not equal\nexpected: %v\ngot:      %v", expected, got)
}

func TestCreateProtoMessage(t *testing.T) {
	runtime := newWireTestRuntime(t)

	t.Run("empty message", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name:   "EmptyRequest",
			Fields: nil,
		}, runtime.getMessageByName("EmptyRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "EmptyRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("single string field", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"name":"hello"}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "ScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("name"), protoref.ValueOfString("hello"))
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"name":"alice","age":30,"score":99.5}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "ScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("name"), protoref.ValueOfString("alice"))
			msg.Set(desc.Fields().ByName("age"), protoref.ValueOfInt32(30))
			msg.Set(desc.Fields().ByName("score"), protoref.ValueOfFloat64(99.5))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("wrapper string value present", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"name":"hello"}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			setWrapperValue(msg, "name", protoref.ValueOfString("hello"))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("wrapper string value absent", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "WrapperScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name", Optional: true},
			},
		}, runtime.getMessageByName("WrapperScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			// absent wrapper means null — no Set call.
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"age":25,"score":3.14}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "WrapperScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			setWrapperValue(msg, "age", protoref.ValueOfInt32(25))
			setWrapperValue(msg, "score", protoref.ValueOfFloat64(3.14))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("repeated strings", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "RepeatedScalarRequest",
			Fields: RPCFields{
				{Name: "tags", ProtoTypeName: DataTypeString, JSONPath: "tags", Repeated: true},
			},
		}, runtime.getMessageByName("RepeatedScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"tags":["foo","bar","baz"]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "RepeatedScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("tags")).List()
			list.Append(protoref.ValueOfString("foo"))
			list.Append(protoref.ValueOfString("bar"))
			list.Append(protoref.ValueOfString("baz"))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("repeated int32s", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "RepeatedScalarRequest",
			Fields: RPCFields{
				{Name: "scores", ProtoTypeName: DataTypeInt32, JSONPath: "scores", Repeated: true},
			},
		}, runtime.getMessageByName("RepeatedScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"scores":[1,2,3]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "RepeatedScalarRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("scores")).List()
			list.Append(protoref.ValueOfInt32(1))
			list.Append(protoref.ValueOfInt32(2))
			list.Append(protoref.ValueOfInt32(3))
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"item":{"id":"1","value":"a"}}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "NestedMessageRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			itemField := desc.Fields().ByName("item")
			item := dynamicpb.NewMessage(itemField.Message())
			item.Set(itemField.Message().Fields().ByName("id"), protoref.ValueOfString("1"))
			item.Set(itemField.Message().Fields().ByName("value"), protoref.ValueOfString("a"))
			msg.Set(itemField, protoref.ValueOfMessage(item))
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"items":[{"id":"1","value":"a"},{"id":"2","value":"b"}]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "NestedMessageRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
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
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("enum field", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumRequest",
			Fields: RPCFields{
				{Name: "status", ProtoTypeName: DataTypeEnum, JSONPath: "status", EnumName: "Status"},
			},
		}, runtime.getMessageByName("EnumRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"status":"ACTIVE"}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "EnumRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("status"), protoref.ValueOfEnum(1))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("repeated enums", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "EnumRequest",
			Fields: RPCFields{
				{Name: "statuses", ProtoTypeName: DataTypeEnum, JSONPath: "statuses", EnumName: "Status", Repeated: true},
			},
		}, runtime.getMessageByName("EnumRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		got, err := wm.createProtoMessage(astjson.MustParse(`{"statuses":["UNSPECIFIED","ACTIVE","INACTIVE"]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "EnumRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			list := msg.Mutable(desc.Fields().ByName("statuses")).List()
			list.Append(protoref.ValueOfEnum(0))
			list.Append(protoref.ValueOfEnum(1))
			list.Append(protoref.ValueOfEnum(2))
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("list wrapper with strings", func(t *testing.T) {
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"optionalTags":["a","b"]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "ListWrapperRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
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
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("nested list wrapper two levels", func(t *testing.T) {
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"tagGroups":[["a","b"],["c"]]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "NestedListRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			tagGroupsField := desc.Fields().ByName("tag_groups")
			lolosDesc := tagGroupsField.Message() // ListOfListOfString

			outerListField := lolosDesc.Fields().ByName("list")
			outerListDesc := outerListField.Message() // ListOfListOfString.List
			outerItemsField := outerListDesc.Fields().ByName("items")
			losDesc := outerItemsField.Message() // ListOfString

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
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("nested list wrapper two levels with messages", func(t *testing.T) {
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"itemGroups":[[{"id":"1","value":"a"},{"id":"2","value":"b"}],[{"id":"3","value":"c"}]]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "NestedListRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			itemGroupsField := desc.Fields().ByName("item_groups")
			loloniDesc := itemGroupsField.Message() // ListOfListOfNestedItem

			outerListField := loloniDesc.Fields().ByName("list")
			outerListDesc := outerListField.Message() // ListOfListOfNestedItem.List
			outerItemsField := outerListDesc.Fields().ByName("items")
			loniDesc := outerItemsField.Message() // ListOfNestedItem

			buildListOfNestedItem := func(items ...struct{ id, value string }) *dynamicpb.Message {
				innerListField := loniDesc.Fields().ByName("list")
				innerListDesc := innerListField.Message()
				nestedItemDesc := innerListDesc.Fields().ByName("items").Message()

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
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"id":"p1","description":"a product","tags":["sale","new"],"price":29.99,"status":"ACTIVE"}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "MixedRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			msg.Set(desc.Fields().ByName("id"), protoref.ValueOfString("p1"))
			setWrapperValue(msg, "description", protoref.ValueOfString("a product"))
			tagsList := msg.Mutable(desc.Fields().ByName("tags")).List()
			tagsList.Append(protoref.ValueOfString("sale"))
			tagsList.Append(protoref.ValueOfString("new"))
			msg.Set(desc.Fields().ByName("price"), protoref.ValueOfFloat64(29.99))
			msg.Set(desc.Fields().ByName("status"), protoref.ValueOfEnum(1))
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"representations":[{"__typename":"Product","id":"1"}]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "LookupProductByIdRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			keysField := desc.Fields().ByName("keys")
			keyDesc := keysField.Message()
			list := msg.Mutable(keysField).List()

			key1 := dynamicpb.NewMessage(keyDesc)
			key1.Set(keyDesc.Fields().ByName("id"), protoref.ValueOfString("1"))
			list.Append(protoref.ValueOfMessage(key1))
		})
		assertProtoMessageEqual(t, expected, got)
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

		got, err := wm.createProtoMessage(astjson.MustParse(`{"representations":[{"__typename":"Product","id":"1"},{"__typename":"Product","id":"2"},{"__typename":"Product","id":"3"}]}`))
		require.NoError(t, err)

		expected := buildExpectedProto(t, runtime, "LookupProductByIdRequest", func(msg *dynamicpb.Message, desc protoref.MessageDescriptor) {
			keysField := desc.Fields().ByName("keys")
			keyDesc := keysField.Message()
			list := msg.Mutable(keysField).List()

			for _, id := range []string{"1", "2", "3"} {
				key := dynamicpb.NewMessage(keyDesc)
				key.Set(keyDesc.Fields().ByName("id"), protoref.ValueOfString(id))
				list.Append(protoref.ValueOfMessage(key))
			}
		})
		assertProtoMessageEqual(t, expected, got)
	})

	t.Run("required field missing returns error", func(t *testing.T) {
		message := compileTestProgramMessage(t, runtime, &RPCMessage{
			Name: "ScalarRequest",
			Fields: RPCFields{
				{Name: "name", ProtoTypeName: DataTypeString, JSONPath: "name"},
			},
		}, runtime.getMessageByName("ScalarRequest"))
		wm := compileTestWireMessage(t, runtime, message)

		_, err := wm.createProtoMessage(astjson.MustParse(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field name is required but has no value")
	})

	t.Run("required list wrapper empty returns error", func(t *testing.T) {
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

		_, err := wm.createProtoMessage(astjson.MustParse(`{"tagGroups":[]}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list is required but has no elements")
	})
}

// TestScalarProtoValue verifies the JSON-to-protoref.Value conversion for each
// supported DataType. The conversion is the leaf shared by every scalar code
// path in wire_proto.go, so locking down its types catches mistakes (e.g.
// truncating ints, sign-extending uints) that the higher-level builders
// would propagate.
func TestScalarProtoValue(t *testing.T) {
	cases := []struct {
		name     string
		dataType DataType
		json     string
		expected protoref.Value
	}{
		{"string", DataTypeString, `"hello"`, protoref.ValueOfString("hello")},
		{"empty string", DataTypeString, `""`, protoref.ValueOfString("")},
		{"int32", DataTypeInt32, `42`, protoref.ValueOfInt32(42)},
		{"int32 negative", DataTypeInt32, `-7`, protoref.ValueOfInt32(-7)},
		{"int64", DataTypeInt64, `9007199254740992`, protoref.ValueOfInt64(9007199254740992)},
		{"uint32", DataTypeUint32, `42`, protoref.ValueOfUint32(42)},
		{"uint64", DataTypeUint64, `9007199254740992`, protoref.ValueOfUint64(9007199254740992)},
		{"float", DataTypeFloat, `1.5`, protoref.ValueOfFloat32(1.5)},
		{"double", DataTypeDouble, `3.14`, protoref.ValueOfFloat64(3.14)},
		{"bool true", DataTypeBool, `true`, protoref.ValueOfBool(true)},
		{"bool false", DataTypeBool, `false`, protoref.ValueOfBool(false)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scalarProtoValue(tc.dataType, astjson.MustParse(tc.json))
			require.NoError(t, err)
			assert.True(t, tc.expected.Equal(got), "expected %v, got %v", tc.expected, got)
		})
	}

	t.Run("unsupported type returns error", func(t *testing.T) {
		_, err := scalarProtoValue(DataTypeUnknown, astjson.MustParse(`null`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported data type")
	})
}

// TestSetProtoFieldFromValue exercises the resolve-context setter that copies
// already-typed protoref.Values into a target message. Both scalar and list
// fields go through it; tested separately so a regression in either branch
// fails before the end-to-end resolve tests do.
func TestSetProtoFieldFromValue(t *testing.T) {
	runtime := newWireTestRuntime(t)

	t.Run("scalar value", func(t *testing.T) {
		rtMsg := runtime.getMessageByName("ScalarRequest")
		require.NotNil(t, rtMsg)
		msg := dynamicpb.NewMessage(rtMsg.desc)
		fd := rtMsg.desc.Fields().ByName("name")

		err := setProtoFieldFromValue(msg, fd, protoref.ValueOfString("hello"))
		require.NoError(t, err)
		assert.Equal(t, "hello", msg.Get(fd).String())
	})

	t.Run("list value copies elements", func(t *testing.T) {
		rtMsg := runtime.getMessageByName("RepeatedScalarRequest")
		require.NotNil(t, rtMsg)
		msg := dynamicpb.NewMessage(rtMsg.desc)
		fd := rtMsg.desc.Fields().ByName("tags")

		// Build a source list to copy from.
		srcMsg := dynamicpb.NewMessage(rtMsg.desc)
		srcList := srcMsg.Mutable(fd).List()
		srcList.Append(protoref.ValueOfString("a"))
		srcList.Append(protoref.ValueOfString("b"))

		err := setProtoFieldFromValue(msg, fd, protoref.ValueOfList(srcList))
		require.NoError(t, err)

		gotList := msg.Get(fd).List()
		require.Equal(t, 2, gotList.Len())
		assert.Equal(t, "a", gotList.Get(0).String())
		assert.Equal(t, "b", gotList.Get(1).String())
	})

	t.Run("non-list value on list field returns error", func(t *testing.T) {
		rtMsg := runtime.getMessageByName("RepeatedScalarRequest")
		require.NotNil(t, rtMsg)
		msg := dynamicpb.NewMessage(rtMsg.desc)
		fd := rtMsg.desc.Fields().ByName("tags")

		err := setProtoFieldFromValue(msg, fd, protoref.ValueOfString("not a list"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "expected list value")
	})
}
