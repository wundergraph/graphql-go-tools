package grpcdatasource

import (
	"bytes"
	"fmt"
	"math"

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
	minBufferSize = 1 << 8 // 256 bytes
)

func compileWireMessageFromRequest(schema *runtimeSchema, request *request) (*wireMessage, error) {
	if request == nil {
		return nil, fmt.Errorf("unable to compile wire message from request: request is nil")
	}

	return compileWireMessage(schema, request.message, make(map[string]*wireMessage))
}

func compileWireMessage(schema *runtimeSchema, msg *programMessage, cycleMap map[string]*wireMessage) (*wireMessage, error) {
	if seen, ok := cycleMap[msg.name]; ok {
		return seen, nil
	}

	if msg == nil {
		return nil, fmt.Errorf("message not found for fetch request")
	}

	messageFields := msg.fields

	wm := &wireMessage{
		fields: make([]wireField, len(messageFields)),
	}

	cycleMap[msg.name] = wm

	for i := range messageFields {
		messageField := messageFields[i]

		wf := wireField{
			number:         messageField.runtime.desc.Number(),
			runtimeMessage: messageField.runtime.message,
			dataType:       messageField.dataType,
			wireType:       getWireType(messageField.runtime.dataType),
			jsonPath:       messageField.jsonPath,
			staticValue:    messageField.staticValue,
			optional:       messageField.optional,
			repeated:       messageField.repeated,
			listMetadata:   messageField.listMetadata,
		}

		if messageField.enumName != "" {
			rtEnum, ok := schema.enumByName[messageField.enumName]
			if !ok {
				return nil, fmt.Errorf("enum not found for name %s", messageField.enumName)
			}

			wf.runtimeEnum = rtEnum
		}

		if messageField.child != nil {
			fieldMessageRuntime := messageField.child.runtime

			// we we are using wrapper messages, they are compiled from the protobuf schema but doesn't match with the RPC planner schema.
			// We need to resolve the correct message from the runtime schema.
			if fieldMessageRuntime.name != messageField.child.name {
				fieldMessageRuntime = schema.getMessageByName(messageField.child.runtime.name)
				if fieldMessageRuntime == nil {
					return nil, fmt.Errorf("message not found for name %s", messageField.child.runtime.name)
				}
			}

			child, err := compileWireMessage(schema, messageField.child, cycleMap)
			if err != nil {
				return nil, err
			}

			wf.child = child
		}

		wf.tag = protowire.AppendTag(nil, wf.number, wf.wireType)
		wm.fields[i] = wf
	}

	return wm, nil
}

// createProtoWire creates a proto wire from the wire plan.
func (w *wireMessage) createProtoWire(data *astjson.Value) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, minBufferSize))
	if err := w.appendProtoWire(buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// appendProtoWire encodes the message fields into the given buffer.
func (w *wireMessage) appendProtoWire(buf *bytes.Buffer, data *astjson.Value) error {
	for _, field := range w.fields {
		if err := field.appendFieldWire(buf, data); err != nil {
			return err
		}
	}
	return nil
}

func (f *wireField) appendFieldWire(buf *bytes.Buffer, data *astjson.Value) error {
	var fieldData *astjson.Value

	switch {
	case f.jsonPath != "":
		fieldData = data.Get(f.jsonPath)
	default:
		fieldData = data
	}

	if !fieldData.Exists() {
		if f.optional {
			return nil
		}

		return fmt.Errorf("field %s is required but has no value", f.jsonPath)
	}

	if f.repeated {
		for _, element := range fieldData.GetArray() {
			err := f.appendFieldValue(buf, element)
			if err != nil {
				return err
			}
		}

		return nil
	}

	if f.listMetadata != nil {
		// TODO: build a wireMessage for the list wrapper and just create the proto wire for it
		//wm := &wireMessage{fields: make([]wireField, 0, 1)}
		return f.appendListFieldValue(buf, fieldData, 0)
	}

	if f.isOptionalScalar() {
		return f.appendOptionalScalarFieldValue(buf, fieldData)
	}

	return f.appendFieldValue(buf, fieldData)
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
func (f *wireField) appendListFieldValue(buf *bytes.Buffer, data *astjson.Value, level int) error {
	if level >= f.listMetadata.NestingLevel {
		f.listMetadata = nil // reset the list metadata to avoid infinite recursion
		return f.appendFieldWire(buf, data)
	}

	md := f.listMetadata.LevelInfo[level]
	level++

	runtimeMsg := f.runtimeMessage
	if runtimeMsg == nil {
		return fmt.Errorf("runtime message not found for field %s", f.jsonPath)
	}

	field, ok := runtimeMsg.fieldsByName["list"]
	if !ok {
		return fmt.Errorf("list field not found for message %s but was expected", runtimeMsg.name)
	}

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

	listBuffer := bytes.NewBuffer(make([]byte, 0, minBufferSize))

	// We will always have a message type here, therefore we must use the bytes type.
	listBuffer.Write(protowire.AppendTag(listBuffer.AvailableBuffer(), field.desc.Number(), protowire.BytesType))

	itemsBuffer := bytes.NewBuffer(make([]byte, 0, minBufferSize))

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
		if err := iwf.appendListFieldValue(itemsBuffer, elements[i], level); err != nil {
			return err
		}
	}

	listBuffer.Write(protowire.AppendVarint(listBuffer.AvailableBuffer(), uint64(itemsBuffer.Len())))
	listBuffer.Write(itemsBuffer.Bytes())

	buf.Write(f.tag)
	buf.Write(protowire.AppendVarint(buf.AvailableBuffer(), uint64(listBuffer.Len())))
	buf.Write(listBuffer.Bytes())

	return nil
}

func (f *wireField) appendOptionalScalarFieldValue(buf *bytes.Buffer, data *astjson.Value) error {
	if f.runtimeMessage == nil {
		return fmt.Errorf("runtime message not found for optional scalar field %s but was expected", f.jsonPath)
	}

	wrapperField, ok := f.runtimeMessage.fieldsByName[knownTypeOptionalFieldValueName]
	if !ok {
		return fmt.Errorf("wrapper field not found for message %s but was expected", f.runtimeMessage.name)
	}

	wf := wireField{
		number:         wrapperField.desc.Number(),
		dataType:       wrapperField.dataType,
		wireType:       getWireType(wrapperField.dataType),
		jsonPath:       f.jsonPath,
		runtimeMessage: wrapperField.message,
	}

	wf.tag = protowire.AppendTag(nil, wf.number, wf.wireType)

	fieldBuf := bytes.NewBuffer(make([]byte, 0, minBufferSize))
	if err := wf.appendFieldValue(fieldBuf, data); err != nil {
		return err
	}

	buf.Write(f.tag)
	buf.Write(protowire.AppendVarint(buf.AvailableBuffer(), uint64(fieldBuf.Len())))
	buf.Write(fieldBuf.Bytes())
	return nil
}

func (f *wireField) isOptionalScalar() bool {
	return f.optional && f.dataType != DataTypeMessage
}

func (f *wireField) appendFieldValue(buf *bytes.Buffer, data *astjson.Value) error {
	if f.child != nil {
		childBuf := bytes.NewBuffer(make([]byte, 0, minBufferSize))
		if err := f.child.appendProtoWire(childBuf, data); err != nil {
			return err
		}
		buf.Write(f.tag)
		buf.Write(protowire.AppendVarint(buf.AvailableBuffer(), uint64(childBuf.Len())))
		buf.Write(childBuf.Bytes())
		return nil
	}

	switch f.wireType {
	case protowire.BytesType:
		buf.Write(f.tag)
		sb := data.GetStringBytes()
		buf.Write(protowire.AppendVarint(buf.AvailableBuffer(), uint64(len(sb))))
		buf.Write(sb)
	case protowire.VarintType:
		value := getUint64Value(data)
		if f.runtimeEnum != nil {
			var err error
			if value, err = f.getEnumValue(data); err != nil {
				return err
			}
		}

		buf.Write(f.tag)
		buf.Write(protowire.AppendVarint(buf.AvailableBuffer(), value))
	case protowire.Fixed64Type:
		buf.Write(f.tag)
		buf.Write(protowire.AppendFixed64(buf.AvailableBuffer(), math.Float64bits(data.GetFloat64())))
	default:
		return fmt.Errorf("unsupported wire type %d", f.wireType)
	}

	return nil
}

func getUint64Value(data *astjson.Value) uint64 {
	switch data.Type() {
	case astjson.TypeNumber:
		return data.GetUint64()
	case astjson.TypeTrue:
		return 1
	default:
		return 0
	}
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
