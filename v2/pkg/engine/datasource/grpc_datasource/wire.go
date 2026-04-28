package grpcdatasource

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

var errShouldSkip = errors.New("skip")

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
	fields      []wireField
	runtime     *runtimeMessage
	oneOfType   OneOfType
	oneOfFields map[string][]wireField
}

type wireField struct {
	tag          []byte
	runtime      *runtimeField
	number       protowire.Number
	dataType     DataType
	wireType     protowire.Type
	runtimeEnum  *runtimeEnum
	staticValue  string
	jsonPath     string
	optional     bool
	repeated     bool
	listMetadata *ListMetadata
	fieldMessage *runtimeMessage
	child        *wireMessage
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
		runtime:   msg.runtime,
		fields:    make([]wireField, len(messageFields)),
		oneOfType: msg.oneOfType,
	}

	cycleMap[msg.name] = wm

	if wm.oneOfType != OneOfTypeNone {
		wm.oneOfFields = make(map[string][]wireField, len(msg.memberTypes))

		for _, memberType := range msg.memberTypes {

			fields, err := compileMessageFields(schema, msg.oneOfFields[memberType], cycleMap)
			if err != nil {
				return nil, err
			}

			wm.oneOfFields[memberType] = fields
		}
	}

	fields, err := compileMessageFields(schema, messageFields, cycleMap)
	if err != nil {
		return nil, err
	}

	wm.fields = fields
	return wm, nil
}

func compileMessageFields(schema *runtimeSchema, messageFields []programField, cycleMap map[string]*wireMessage) ([]wireField, error) {
	if len(messageFields) == 0 {
		return nil, nil
	}

	fields := make([]wireField, len(messageFields))

	for i := range messageFields {
		messageField := messageFields[i]

		wf := wireField{
			runtime:      messageField.runtime,
			number:       messageField.runtime.desc.Number(),
			fieldMessage: messageField.runtime.message,
			dataType:     messageField.dataType,
			wireType:     getWireType(messageField.runtime.dataType),
			jsonPath:     messageField.jsonPath,
			staticValue:  messageField.staticValue,
			optional:     messageField.optional,
			repeated:     messageField.repeated,
			listMetadata: messageField.listMetadata,
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
		fields[i] = wf
	}

	return fields, nil
}

func (w *wireMessage) createProtoWireWithContext(a arena.Arena, data *astjson.Value, context *fetchRequestContext, contextMessage protoref.Message) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, minBufferSize))

	contextValues := make([]map[string]protoref.Value, 0)
	for _, contextField := range context.fields {
		values := resolveContextDataForPath(contextMessage, contextField.p)

		for index, value := range values {
			if index >= len(contextValues) {
				contextValues = append(contextValues, make(map[string]protoref.Value))
			}

			contextValues[index][contextField.jsonName] = value
		}
	}

	if len(contextValues) == 0 {
		return nil, errShouldSkip
	}

	contextVariables := astjson.ArrayValue(a)
	arrayIndex := 0
	for _, contextValues := range contextValues {
		contextVariable := astjson.ObjectValue(a)
		for fieldName, contextValue := range contextValues {
			contextVariable.Set(a, fieldName, convertProtoRefValue(a, contextValue))
		}

		contextVariables.SetArrayItem(a, arrayIndex, contextVariable)
		arrayIndex++
	}

	for _, field := range w.fields {
		if field.runtime.name == "context" {
			if err := field.appendFieldWire(buf, contextVariables); err != nil {
				return nil, err
			}

			continue
		}

		if err := field.appendFieldWire(buf, data); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
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
	if w.oneOfType != OneOfTypeNone {
		if !data.Exists("__typename") {
			return fmt.Errorf("__typename is required for oneof fields")
		}

		typeName := string(data.Get("__typename").GetStringBytes())

		oneOfDescriptor := w.oneOfTypeDecriptor()
		if oneOfDescriptor == nil {
			return fmt.Errorf("oneof descriptor not found for message %s", w.runtime.name)
		}

		fields := oneOfDescriptor.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			if field.Kind() != protoref.MessageKind {
				continue
			}

			if field.Message().Name() == protoref.Name(typeName) {
				fieldNumber := field.Number()
				buf.Write(protowire.AppendTag(buf.AvailableBuffer(), fieldNumber, protowire.BytesType))
				break
			}
		}

		oneOfFields := w.oneOfFields[typeName]
		fieldsBuffer := bytes.NewBuffer(make([]byte, 0, minBufferSize))
		for _, field := range oneOfFields {
			if err := field.appendFieldWire(fieldsBuffer, data); err != nil {
				return err
			}
		}

		buf.Write(protowire.AppendBytes(buf.AvailableBuffer(), fieldsBuffer.Bytes()))
		fieldsBuffer.Reset()
		return nil
	}

	for _, field := range w.fields {
		if err := field.appendFieldWire(buf, data); err != nil {
			return err
		}
	}
	return nil
}

func (w *wireMessage) oneOfTypeDecriptor() protoref.OneofDescriptor {
	oneOfs := w.runtime.desc.Oneofs()
	if oneOfs == nil || oneOfs.Len() == 0 {
		return nil
	}

	switch w.oneOfType {
	case OneOfTypeInterface:
		return oneOfs.ByName(protoref.Name("instance"))
	case OneOfTypeUnion:
		return oneOfs.ByName(protoref.Name("value"))
	default:
		return nil
	}
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

	runtimeMsg := f.fieldMessage
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
			number:       itemsField.desc.Number(),
			dataType:     f.dataType,
			wireType:     getWireType(itemsField.dataType),
			fieldMessage: itemsField.message,
			listMetadata: f.listMetadata,
			child:        f.child,
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
	if f.fieldMessage == nil {
		return fmt.Errorf("runtime message not found for optional scalar field %s but was expected", f.jsonPath)
	}

	wrapperField, ok := f.fieldMessage.fieldsByName[knownTypeOptionalFieldValueName]
	if !ok {
		return fmt.Errorf("wrapper field not found for message %s but was expected", f.fieldMessage.name)
	}

	wf := wireField{
		number:       wrapperField.desc.Number(),
		dataType:     wrapperField.dataType,
		wireType:     getWireType(wrapperField.dataType),
		jsonPath:     f.jsonPath,
		fieldMessage: wrapperField.message,
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
		buf.Write(protowire.AppendBytes(buf.AvailableBuffer(), childBuf.Bytes()))
		childBuf.Reset()
		return nil
	}

	switch f.wireType {
	case protowire.BytesType:
		buf.Write(f.tag)
		buf.Write(protowire.AppendBytes(buf.AvailableBuffer(), getBytesValue(data)))
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

func getBytesValue(data *astjson.Value) []byte {
	switch data.Type() {
	case astjson.TypeString:
		return data.GetStringBytes()
	case astjson.TypeNumber:
		num := data.GetUint64()
		return []byte(strconv.FormatUint(num, 10))
	case astjson.TypeTrue:
		return []byte{1}
	case astjson.TypeFalse:
		return []byte{0}
	default:
		return nil
	}
}

func (f *wireField) getEnumValue(data *astjson.Value) (uint64, error) {
	switch data.Type() {
	case astjson.TypeNumber:
		return data.GetUint64(), nil
	case astjson.TypeString:
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

	default:
		return 0, fmt.Errorf("unsupported enum type %s", data.Type())
	}
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

// resolveContextDataForPath resolves the data for a given path in the context message.
func resolveContextDataForPath(message protoref.Message, path ast.Path) []protoref.Value {
	if path.Len() == 0 {
		return nil
	}

	segment := path[0]
	path = path[1:]

	msg, fd := getMessageField(message, segment.FieldName.String())
	if !msg.IsValid() {
		return nil
	}

	if fd.IsList() {
		return resolveListDataForPath(msg.List(), fd, path)
	}

	return resolveDataForPath(msg.Message(), path)
}

// resolveListDataForPath resolves the data for a given path in a list message.
func resolveListDataForPath(message protoref.List, fd protoref.FieldDescriptor, path ast.Path) []protoref.Value {
	if !message.IsValid() {
		return nil
	}

	if path.Len() == 0 {
		return nil
	}

	result := make([]protoref.Value, 0, message.Len())

	for i := range message.Len() {
		item := message.Get(i)

		switch fd.Kind() {
		case protoref.MessageKind:
			values := resolveDataForPath(item.Message(), path)

			for _, val := range values {
				if list, isList := val.Interface().(protoref.List); isList {
					values := resolveListDataForPath(list, fd, path[1:])
					result = append(result, values...)
					continue
				} else {
					result = append(result, val)
				}
			}

		default:
			result = append(result, item)
		}
	}

	return result
}

// resolveDataForPath resolves the data for a given path in a message.
func resolveDataForPath(message protoref.Message, path ast.Path) []protoref.Value {
	if !message.IsValid() {
		return nil
	}

	if path.Len() == 0 {
		return nil
	}

	segment := path[0]

	if fn := segment.FieldName.String(); strings.HasPrefix(fn, "@") {
		list := resolveUnderlyingList(message, fn)

		result := make([]protoref.Value, 0, len(list))
		for _, item := range list {
			result = append(result, resolveDataForPath(item.Message(), path[1:])...)
		}

		return result
	}

	field, fd := getMessageField(message, segment.FieldName.String())
	if !field.IsValid() {
		return nil
	}

	switch fd.Kind() {
	case protoref.MessageKind:
		if fd.IsList() {
			// We always return a list value here even if the list is empty.
			// Repeatable fields in protobuf are always at least an empty list.
			return []protoref.Value{protoref.ValueOfList(field.List())}
		}

		if !field.Message().IsValid() {
			return nil
		}

		return resolveDataForPath(field.Message(), path[1:])
	default:
		return []protoref.Value{field}
	}
}

// getMessageField gets the field from the message by its name.
func getMessageField(message protoref.Message, fieldName string) (protoref.Value, protoref.FieldDescriptor) {
	fd := message.Descriptor().Fields().ByName(protoref.Name(fieldName))
	if fd == nil {
		return protoref.Value{}, nil
	}

	return message.Get(fd), fd
}

// resolveUnderlyingList resolves the underlying list message from a nested list message.
//
//	message ListOfFloat {
//	  message List {
//	    repeated double items = 1;
//	  }
//	  List list = 1;
//	}
func resolveUnderlyingList(msg protoref.Message, fieldName string) []protoref.Value {
	nestingLevel := 0
	for _, char := range fieldName {
		if char != '@' {
			break
		}
		nestingLevel++
	}

	listFieldValue := msg.Get(msg.Descriptor().Fields().ByName(protoref.Name(fieldName[nestingLevel:])))
	if !listFieldValue.IsValid() {
		return nil
	}

	return resolveUnderlyingListItems(listFieldValue, nestingLevel)

}

// resolveUnderlyingListItems resolves the items in a list message.
//
//	message ListOfFloat {
//	  message List {
//	    repeated double items = 1;
//	  }
//	  List list = 1;
//	}
func resolveUnderlyingListItems(value protoref.Value, nestingLevel int) []protoref.Value {
	// The field number of the list and items field in the message
	const listAndItemsFieldNumber = 1
	msg := value.Message()
	fd := msg.Descriptor().Fields().ByNumber(listAndItemsFieldNumber)
	if fd == nil {
		return nil
	}

	listMsg := msg.Get(fd)
	if !listMsg.IsValid() {
		return nil
	}

	itemsValue := listMsg.Message().Get(listMsg.Message().Descriptor().Fields().ByNumber(listAndItemsFieldNumber))
	if !itemsValue.IsValid() {
		return nil
	}

	itemsList := itemsValue.List()
	itemsListLen := itemsList.Len()
	if itemsListLen == 0 {
		return nil
	}

	if nestingLevel > 1 {
		items := make([]protoref.Value, 0, itemsListLen)
		for i := 0; i < itemsListLen; i++ {
			items = append(items, resolveUnderlyingListItems(itemsList.Get(i), nestingLevel-1)...)
		}

		return items
	}

	result := make([]protoref.Value, itemsListLen)
	for i := 0; i < itemsListLen; i++ {
		result[i] = itemsList.Get(i)
	}

	return result
}

func convertProtoRefValue(a arena.Arena, value protoref.Value) *astjson.Value {
	switch t := value.Interface().(type) {
	case nil:
		return astjson.NullValue
	case bool:
		if t {
			return astjson.TrueValue(a)
		}
		return astjson.FalseValue(a)
	case int32:
		return astjson.IntValue(a, int(t))
	case int64:
		return astjson.NumberValue(a, strconv.FormatInt(t, 10))
	case uint32:
		return astjson.NumberValue(a, strconv.FormatUint(uint64(t), 10))
	case uint64:
		return astjson.NumberValue(a, strconv.FormatUint(t, 10))
	case float32:
		return astjson.FloatValue(a, float64(t))
	case float64:
		return astjson.FloatValue(a, t)
	case string:
		return astjson.StringValue(a, t)
	case []byte:
		return astjson.StringValueBytes(a, t)
	case protoref.EnumNumber:
		return astjson.IntValue(a, int(t))
	case protoref.List:
		av := astjson.ArrayValue(a)
		for i := range t.Len() {
			item := t.Get(i)
			av.SetArrayItem(a, i, convertProtoRefValue(a, item))
		}
		return av
	case protoref.Message:
		ov := astjson.ObjectValue(a)
		desc := t.Descriptor()
		for i := range desc.Fields().Len() {
			fd := desc.Fields().Get(i)
			value := t.Get(fd)
			ov.Set(a, string(fd.Name()), convertProtoRefValue(a, value))
		}
		return ov
	default:
		fmt.Println("unsupported type", reflect.TypeOf(t).Name())
		return astjson.NullValue
	}
}
