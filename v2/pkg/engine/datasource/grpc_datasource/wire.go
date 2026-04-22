package grpcdatasource

import (
	"bytes"
	"fmt"
	"math"
	"sync"

	"github.com/wundergraph/astjson"
	"google.golang.org/protobuf/encoding/protowire"
)

type PreWiredInputMessage struct {
	size   int
	buffer []byte
}

func NewPreWiredInputMessage(buffer []byte) *PreWiredInputMessage {
	return &PreWiredInputMessage{
		size:   len(buffer),
		buffer: buffer,
	}
}

func (c *PreWiredInputMessage) wire() ([]byte, error) {
	if c.buffer == nil {
		return nil, fmt.Errorf("connect message not initialized")
	}

	return c.buffer, nil
}

type wireMessage struct {
	fields    []wireField
	oneOfType OneOfType
}

type wireField struct {
	tag            []byte
	number         protowire.Number
	dataType       DataType
	wireType       protowire.Type
	runtimeMessage *runtimeMessage
	runtimeEnum    *runtimeEnum
	staticValue    string
	jsonPath       string
	optional       bool
	repeated       bool
	listMetadata   *ListMetadata
	child          *wireMessage
}

const (
	minBufferSize = 1 << 9  // 512 bytes
	maxBufferSize = 1 << 20 // 1MiB

	defaultBufferCount = 10
)

// TODO: This should use an area instead
type bufferPool struct {
	pool        sync.Pool
	defaultSize int
}

func newMempool(size int) *bufferPool {
	if size <= 0 || size < minBufferSize || size > maxBufferSize {
		size = minBufferSize
	}

	return &bufferPool{
		pool: sync.Pool{
			New: func() any {
				return bytes.NewBuffer(make([]byte, 0, size))
			},
		},
		defaultSize: size,
	}
}

func (b *bufferPool) Get() *bytes.Buffer {
	buf := b.pool.Get().(*bytes.Buffer)
	buf.Reset()
	if buf.Cap() < b.defaultSize {
		buf.Grow(b.defaultSize - buf.Cap())
	}

	return buf
}

func (b *bufferPool) Put(buf *bytes.Buffer) {
	b.pool.Put(buf)
}

type wireBuilder struct {
	pool    *bufferPool
	buffers []*bytes.Buffer
}

func newWireBuilder(size int) *wireBuilder {
	return &wireBuilder{
		pool:    newMempool(size),
		buffers: make([]*bytes.Buffer, 0, defaultBufferCount),
	}
}

func (w *wireBuilder) Reset() {
	for _, buf := range w.buffers {
		w.pool.Put(buf)
	}
	w.buffers = w.buffers[:0]
}

func compileWireMessage(runtime *runtimeSchema, rpcMessage *RPCMessage, message *runtimeMessage) (*wireMessage, error) {
	if message == nil {
		return nil, fmt.Errorf("message not found for fetch request")
	}
	msg := &wireMessage{
		fields: make([]wireField, len(rpcMessage.Fields)),
	}

	// TODO: This is possible for `@requires` fields, but not yet supported.
	if rpcMessage.OneOfType != OneOfTypeNone {
		return nil, fmt.Errorf("oneof type not supported yet")
	}

	for i := range rpcMessage.Fields {
		rpcField := &rpcMessage.Fields[i]

		field, ok := message.fieldsByName[rpcField.Name]
		if !ok {
			return nil, fmt.Errorf("field not found for name %s", rpcField.Name)
		}

		wf := wireField{
			number:         field.desc.Number(),
			runtimeMessage: field.message,
			dataType:       rpcField.ProtoTypeName,
			wireType:       getWireType(field.dataType),
			jsonPath:       rpcField.JSONPath,
			staticValue:    rpcField.StaticValue,
			optional:       rpcField.Optional,
			repeated:       rpcField.Repeated,
			listMetadata:   rpcField.ListMetadata,
		}

		if rpcField.EnumName != "" {
			rtEnum, ok := runtime.enumByName[rpcField.EnumName]
			if !ok {
				return nil, fmt.Errorf("enum not found for name %s", rpcField.EnumName)
			}

			wf.runtimeEnum = rtEnum
		}

		if rpcField.Message != nil {
			fieldMessage := field.message
			// we we are using wrapper messages, they are compiled from the protobuf schema but doesn't match with the RPC planner schema.
			// We need to resolve the correct message from the runtime schema.
			if rpcField.Message.Name != fieldMessage.name {
				fieldMessage = runtime.getMessageByName(rpcField.Message.Name)
				if fieldMessage == nil {
					return nil, fmt.Errorf("message not found for name %s", rpcField.Message.Name)
				}
			}

			child, err := compileWireMessage(runtime, rpcField.Message, fieldMessage)
			if err != nil {
				return nil, err
			}

			wf.child = child
		}

		wf.tag = protowire.AppendTag(nil, wf.number, wf.wireType)
		msg.fields[i] = wf
	}

	return msg, nil
}

// createProtoWire creates a proto wire from the wire plan.
func (w *wireMessage) createProtoWire(builder *wireBuilder, data *astjson.Value) ([]byte, error) {
	// TODO: Use arena or a global buffer pool
	buf := builder.pool.Get()

	for _, field := range w.fields {
		err := field.appendFieldWire(builder, buf, data)
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (f *wireField) appendFieldWire(builder *wireBuilder, buf *bytes.Buffer, data *astjson.Value) error {
	var fieldData *astjson.Value

	if f.jsonPath == "" {
		fieldData = data
	} else {
		fieldData = data.Get(f.jsonPath)
	}

	if !fieldData.Exists() {
		if f.optional {
			return nil
		}

		return fmt.Errorf("field %s is required but has no value", f.jsonPath)
	}

	if f.repeated {
		for _, element := range fieldData.GetArray() {
			err := f.appendFieldValue(builder, buf, element)
			if err != nil {
				return err
			}
		}

		return nil
	}

	if f.listMetadata != nil {
		// TODO: build a wireMessage for the list wrapper and just create the proto wire for it
		//wm := &wireMessage{fields: make([]wireField, 0, 1)}
		return f.appendListFieldValue(builder, buf, fieldData, 0)
	}

	if f.isOptionalScalar() {
		return f.appendOptionalScalarFieldValue(builder, buf, fieldData)
	}

	return f.appendFieldValue(builder, buf, fieldData)
}

// appendListFieldValue appends the list value to the buffer.
// This is used for lists and nested lists which are defined as wrapper messages.
//
// Example:
// ```proto
//
//	message ListOfString {
//		message List {
//		  repeated string items = 1;
//		}
//		List list = 1;
//	}
//
// ```
func (f *wireField) appendListFieldValue(builder *wireBuilder, buf *bytes.Buffer, data *astjson.Value, level int) error {
	if level >= f.listMetadata.NestingLevel {
		f.listMetadata = nil // reset the list metadata to avoid infinite recursion
		return f.appendFieldWire(builder, buf, data)
	}

	md := f.listMetadata.LevelInfo[level]
	level++

	runtimeMsg := f.runtimeMessage
	if runtimeMsg == nil {
		return fmt.Errorf("runtime message not found for field %s", f.jsonPath)
	}

	listBuffer := builder.pool.Get()
	defer builder.pool.Put(listBuffer)

	field, ok := runtimeMsg.fieldsByName["list"]
	if !ok {
		return fmt.Errorf("list field not found for message %s but was expected", runtimeMsg.name)
	}

	// We will always have a message type here, therefore we must use the bytes type.
	listBuffer.Write(protowire.AppendTag(nil, field.desc.Number(), protowire.BytesType))

	listMessage := field.message
	if listMessage == nil {
		return fmt.Errorf("expected nested message type for list wrapper field but the field %s doesn't have a message", f.jsonPath)
	}

	itemsField, ok := listMessage.fieldsByName["items"]
	if !ok {
		return fmt.Errorf("items field not found for message %s but was expected", listMessage.name)
	}

	elements := data.GetArray()
	if len(elements) == 0 && !md.Optional {
		return fmt.Errorf("list is required but has no elements")
	}

	itemsBuffer := builder.pool.Get()
	defer builder.pool.Put(itemsBuffer)

	for i := range elements {
		iwf := wireField{
			number:         itemsField.desc.Number(),
			dataType:       f.dataType,
			wireType:       getWireType(itemsField.dataType),
			runtimeMessage: itemsField.message,
			listMetadata:   f.listMetadata,
			child:          f.child,
		}

		iwf.tag = protowire.AppendTag(nil, iwf.number, iwf.wireType)
		if err := iwf.appendListFieldValue(builder, itemsBuffer, elements[i], level); err != nil {
			return err
		}
	}

	listBuffer.Write(protowire.AppendBytes(nil, itemsBuffer.Bytes()))

	buf.Write(f.tag)
	buf.Write(protowire.AppendBytes(nil, listBuffer.Bytes()))

	return nil
}

func (f *wireField) appendOptionalScalarFieldValue(builder *wireBuilder, buf *bytes.Buffer, data *astjson.Value) error {
	if f.runtimeMessage == nil {
		return fmt.Errorf("runtime message not found for optional scalar field %s but was expected", f.jsonPath)
	}

	wrapperField, ok := f.runtimeMessage.fieldsByName[knownTypeOptionalFieldValueName]
	if !ok {
		return fmt.Errorf("wrapper field not found for message %s but was expected", f.runtimeMessage.name)
	}

	fieldBuf := builder.pool.Get()
	defer builder.pool.Put(fieldBuf)

	wf := wireField{
		number:         wrapperField.desc.Number(),
		dataType:       wrapperField.dataType,
		wireType:       getWireType(wrapperField.dataType),
		jsonPath:       f.jsonPath,
		runtimeMessage: wrapperField.message,
	}

	wf.tag = protowire.AppendTag(nil, wf.number, wf.wireType)
	err := wf.appendFieldValue(builder, fieldBuf, data)
	if err != nil {
		return err
	}

	buf.Write(f.tag)
	buf.Write(protowire.AppendBytes(nil, fieldBuf.Bytes()))
	return nil
}

func (f *wireField) isOptionalScalar() bool {
	return f.optional && f.dataType != DataTypeMessage
}

func (f *wireField) appendFieldValue(builder *wireBuilder, buf *bytes.Buffer, data *astjson.Value) error {
	if f.child != nil {
		childWire, err := f.child.createProtoWire(builder, data)
		if err != nil {
			return err
		}

		buf.Write(f.tag)
		buf.Write(protowire.AppendBytes(nil, childWire))
		return nil
	}

	switch f.wireType {
	case protowire.BytesType:
		buf.Write(f.tag)
		buf.Write(protowire.AppendBytes(nil, data.GetStringBytes()))
	case protowire.VarintType:
		value := data.GetUint64()
		if f.runtimeEnum != nil {
			var err error
			if value, err = f.getEnumValue(data); err != nil {
				return err
			}
		}

		buf.Write(f.tag)
		buf.Write(protowire.AppendVarint(nil, value))
	case protowire.Fixed64Type:
		buf.Write(f.tag)
		buf.Write(protowire.AppendFixed64(nil, math.Float64bits(data.GetFloat64())))
	default:
		return fmt.Errorf("unsupported wire type %d", f.wireType)
	}

	return nil
}

func (f *wireField) getEnumValue(data *astjson.Value) (uint64, error) {
	enumValueName := data.GetStringBytes()
	if len(enumValueName) == 0 {
		return 0, fmt.Errorf("enum value name is required for enum field %s", f.jsonPath)
	}

	ev, found := f.runtimeEnum.valuesByName[string(enumValueName)]
	if !found {
		return 0, fmt.Errorf("enum value not found for name %s", string(enumValueName))
	}

	if ev.value < 0 {
		return 0, fmt.Errorf("enum value %s is negative for enum field %s", string(enumValueName), f.jsonPath)
	}

	return uint64(ev.value), nil
}

func getWireType(dataType DataType) protowire.Type {
	switch dataType {
	case DataTypeString, DataTypeBytes:
		return protowire.BytesType
	case DataTypeInt32, DataTypeInt64, DataTypeUint32, DataTypeUint64:
		return protowire.VarintType
	case DataTypeFloat, DataTypeDouble:
		return protowire.Fixed64Type
	case DataTypeMessage:
		return protowire.BytesType
	default:
		return protowire.VarintType
	}
}
