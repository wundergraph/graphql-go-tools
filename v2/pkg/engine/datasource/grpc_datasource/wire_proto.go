package grpcdatasource

import (
	"errors"
	"fmt"

	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// createProtoMessage builds a dynamicpb message from the wire plan, populating it
// from the given JSON variables. It is the proto-message counterpart of
// createProtoWire — same input, same plan, but returns a protoref.Message ready
// to be marshaled by callers that need real proto messages (e.g. the Connect client).
func (w *wireMessage) createProtoMessage(data *astjson.Value) (protoref.Message, error) {
	msg := w.runtime.newEmptyMessage()
	if err := w.appendProtoMessage(msg, data); err != nil {
		return nil, err
	}
	return msg, nil
}

// createProtoMessageWithContext builds the input message for a CallKindResolve
// fetch. It resolves context values from the upstream contextMessage, populates
// the synthetic context list on the root message, and fills any remaining
// fields (e.g. field_args) from the request variables.
//
// Returns errShouldSkip if no context values could be resolved — matching the
// wire path behavior.
func (w *wireMessage) createProtoMessageWithContext(_ arena.Arena, data *astjson.Value, context *fetchRequestContext, contextMessage protoref.Message) (protoref.Message, error) {
	contextValues := make([]map[string]protoref.Value, 0)
	for _, contextField := range context.fields {
		values := resolveContextDataForPath(contextMessage, contextField.resolvePath)
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

	rootMsg := w.runtime.newEmptyMessage()

	contextRuntimeField, ok := w.runtime.fieldsByName[contextFieldName]
	if !ok {
		return nil, fmt.Errorf("context field not found in message %s", w.runtime.name)
	}
	contextElementType := contextRuntimeField.message
	if contextElementType == nil {
		return nil, fmt.Errorf("context element message not found for field %s", contextRuntimeField.name)
	}

	contextList := rootMsg.Mutable(contextRuntimeField.desc).List()
	for _, values := range contextValues {
		elem := contextList.NewElement()
		elemMsg := elem.Message()
		for fieldName, value := range values {
			fd, ok := contextElementType.fieldsByName[fieldName]
			if !ok {
				return nil, fmt.Errorf("context field %s not found in message %s", fieldName, contextElementType.name)
			}
			if err := setProtoFieldFromValue(elemMsg, fd.desc, value); err != nil {
				return nil, err
			}
		}
		contextList.Append(elem)
	}

	for _, field := range w.fields {
		if field.runtime.name == contextFieldName {
			continue
		}
		if err := field.setProtoField(rootMsg, data); err != nil {
			return nil, err
		}
	}

	return rootMsg, nil
}

// appendProtoMessage populates msg from data, dispatching to oneof handling
// when the message is a union/interface. Mirrors appendProtoWire.
func (w *wireMessage) appendProtoMessage(msg protoref.Message, data *astjson.Value) error {
	if w.oneOfType != OneOfTypeNone {
		return w.appendOneOfProto(msg, data)
	}

	for _, field := range w.fields {
		if err := field.setProtoField(msg, data); err != nil {
			return err
		}
	}
	return nil
}

// appendOneOfProto resolves the concrete subtype via __typename and populates
// the matching oneof variant. Mirrors the oneof branch in appendProtoWire.
func (w *wireMessage) appendOneOfProto(msg protoref.Message, data *astjson.Value) error {
	if !data.Exists("__typename") {
		return fmt.Errorf("__typename is required for oneof fields")
	}
	typeName := string(data.Get("__typename").GetStringBytes())

	oneOfDescriptor := w.oneOfTypeDecriptor()
	if oneOfDescriptor == nil {
		return fmt.Errorf("oneof descriptor not found for message %s", w.runtime.name)
	}

	var matched protoref.FieldDescriptor
	oneOfDescFields := oneOfDescriptor.Fields()
	for i := range oneOfDescFields.Len() {
		field := oneOfDescFields.Get(i)
		if field.Kind() != protoref.MessageKind {
			continue
		}
		if field.Message().Name() == protoref.Name(typeName) {
			matched = field
			break
		}
	}
	if matched == nil {
		// No matching member type — leave the oneof unset.
		return nil
	}

	subMsg := dynamicpb.NewMessage(matched.Message())
	for _, oneOfField := range w.oneOfFields[typeName] {
		if err := oneOfField.setProtoField(subMsg, data); err != nil {
			return err
		}
	}
	msg.Set(matched, protoref.ValueOfMessage(subMsg))
	return nil
}

// setProtoField is the proto-message counterpart of appendFieldWire. It reads
// the field data via the JSON path and dispatches to the right writer for
// repeated, list-wrapper, optional-scalar, message, enum, or scalar fields.
func (f *wireField) setProtoField(msg protoref.Message, data *astjson.Value) error {
	var fieldData *astjson.Value
	if f.jsonPath != "" {
		fieldData = data.Get(f.jsonPath)
	} else {
		fieldData = data
	}

	if !fieldData.Exists() {
		if f.optional {
			return nil
		}
		return fmt.Errorf("field %s is required but has no value", f.jsonPath)
	}

	fd := f.runtime.desc

	if f.repeated {
		return f.appendProtoRepeated(msg, fd, fieldData)
	}

	if f.listMetadata != nil {
		return f.appendProtoListWrapper(msg, fd, fieldData)
	}

	if f.isOptionalScalar() {
		return f.setProtoOptionalScalar(msg, fd, fieldData)
	}

	return f.setProtoLeafField(msg, fd, fieldData)
}

// setProtoLeafField handles a single (non-repeated, non-wrapper) field,
// including nested messages and enums.
func (f *wireField) setProtoLeafField(msg protoref.Message, fd protoref.FieldDescriptor, data *astjson.Value) error {
	if f.child != nil {
		subMsg := msg.Mutable(fd).Message()
		return f.child.appendProtoMessage(subMsg, data)
	}
	if f.runtimeEnum != nil {
		num, err := f.getEnumValue(data)
		if err != nil {
			return err
		}
		msg.Set(fd, protoref.ValueOfEnum(protoref.EnumNumber(num)))
		return nil
	}
	val, err := scalarProtoValue(f.dataType, data)
	if err != nil {
		return err
	}
	msg.Set(fd, val)
	return nil
}

// appendProtoRepeated handles plain `repeated` fields by iterating the JSON
// array and appending one entry per element.
func (f *wireField) appendProtoRepeated(msg protoref.Message, fd protoref.FieldDescriptor, data *astjson.Value) error {
	elements := data.GetArray()
	if len(elements) == 0 {
		return nil
	}

	list := msg.Mutable(fd).List()
	for _, element := range elements {
		switch {
		case f.child != nil:
			elem := list.NewElement()
			if err := f.child.appendProtoMessage(elem.Message(), element); err != nil {
				return err
			}
			list.Append(elem)
		case f.runtimeEnum != nil:
			num, err := f.getEnumValue(element)
			if err != nil {
				return err
			}
			list.Append(protoref.ValueOfEnum(protoref.EnumNumber(num)))
		default:
			val, err := scalarProtoValue(f.dataType, element)
			if err != nil {
				return err
			}
			list.Append(val)
		}
	}
	return nil
}

// setProtoOptionalScalar wraps a nullable scalar in its wrapper message
// (e.g. google.protobuf.StringValue), mirroring appendOptionalScalarFieldValue.
func (f *wireField) setProtoOptionalScalar(msg protoref.Message, fd protoref.FieldDescriptor, data *astjson.Value) error {
	if f.fieldMessage == nil {
		return fmt.Errorf("runtime message not found for optional scalar field %s but was expected", f.jsonPath)
	}
	wrapperField, ok := f.fieldMessage.fieldsByName[knownTypeOptionalFieldValueName]
	if !ok {
		return fmt.Errorf("wrapper field not found for message %s but was expected", f.fieldMessage.name)
	}

	wrapperMsg := msg.Mutable(fd).Message()
	val, err := scalarProtoValue(wrapperField.dataType, data)
	if err != nil {
		return err
	}
	wrapperMsg.Set(wrapperField.desc, val)
	return nil
}

// appendProtoListWrapper handles list-wrapper messages (used for nullable and
// nested lists). It allocates the outer wrapper and delegates per-level walking
// to traverseProtoList.
func (f *wireField) appendProtoListWrapper(msg protoref.Message, fd protoref.FieldDescriptor, data *astjson.Value) error {
	if f.fieldMessage == nil {
		return fmt.Errorf("runtime message not found for list wrapper field %s", f.jsonPath)
	}
	wrapper := msg.Mutable(fd).Message()
	return f.traverseProtoList(wrapper, f.fieldMessage, 0, data)
}

// traverseProtoList walks a list-wrapper message tree (e.g. ListOfString { List
// list = 1; } or its nested-list variants) and populates each level from the
// JSON array. Mirrors appendListFieldValue.
func (f *wireField) traverseProtoList(rootMsg protoref.Message, wrapperRT *runtimeMessage, level int, data *astjson.Value) error {
	listRTField, ok := wrapperRT.fieldsByName["list"]
	if !ok {
		return fmt.Errorf("list field not found for message %s", wrapperRT.name)
	}
	listMessageRT := listRTField.message
	if listMessageRT == nil {
		return fmt.Errorf("expected nested message for list wrapper but field %s has none", f.jsonPath)
	}
	itemsRTField, ok := listMessageRT.fieldsByName["items"]
	if !ok {
		return fmt.Errorf("items field not found for message %s", listMessageRT.name)
	}

	md := f.listMetadata.LevelInfo[level]
	elements := data.GetArray()

	// Allocate the inner "list" message so the wrapper is marked as set,
	// matching the wire path which always emits the inner tag.
	innerMsg := rootMsg.Mutable(listRTField.desc).Message()

	if len(elements) == 0 {
		if !md.Optional {
			return fmt.Errorf("list is required but has no elements")
		}
		return nil
	}

	itemsList := innerMsg.Mutable(itemsRTField.desc).List()

	if level+1 >= f.listMetadata.NestingLevel {
		// Leaf level — items are scalars, enums, or messages.
		for _, element := range elements {
			switch {
			case f.child != nil:
				elem := itemsList.NewElement()
				if err := f.child.appendProtoMessage(elem.Message(), element); err != nil {
					return err
				}
				itemsList.Append(elem)
			case f.runtimeEnum != nil:
				num, err := f.getEnumValue(element)
				if err != nil {
					return err
				}
				itemsList.Append(protoref.ValueOfEnum(protoref.EnumNumber(num)))
			default:
				val, err := scalarProtoValue(itemsRTField.dataType, element)
				if err != nil {
					return err
				}
				itemsList.Append(val)
			}
		}
		return nil
	}

	// Recursive level — each item is itself a list wrapper one level deeper.
	nextWrapperRT := itemsRTField.message
	if nextWrapperRT == nil {
		return fmt.Errorf("nested list wrapper missing message for field %s", f.jsonPath)
	}
	for _, element := range elements {
		elem := itemsList.NewElement()
		if err := f.traverseProtoList(elem.Message(), nextWrapperRT, level+1, element); err != nil {
			return err
		}
		itemsList.Append(elem)
	}
	return nil
}

// scalarProtoValue converts a JSON value to the protoref.Value matching dt.
func scalarProtoValue(dt DataType, data *astjson.Value) (protoref.Value, error) {
	switch dt {
	case DataTypeString:
		return protoref.ValueOfString(string(data.GetStringBytes())), nil
	case DataTypeBytes:
		return protoref.ValueOfBytes(data.GetStringBytes()), nil
	case DataTypeInt32:
		return protoref.ValueOfInt32(int32(data.GetInt64())), nil
	case DataTypeInt64:
		return protoref.ValueOfInt64(data.GetInt64()), nil
	case DataTypeUint32:
		return protoref.ValueOfUint32(uint32(data.GetUint64())), nil
	case DataTypeUint64:
		return protoref.ValueOfUint64(data.GetUint64()), nil
	case DataTypeFloat:
		return protoref.ValueOfFloat32(float32(data.GetFloat64())), nil
	case DataTypeDouble:
		return protoref.ValueOfFloat64(data.GetFloat64()), nil
	case DataTypeBool:
		return protoref.ValueOfBool(data.GetBool()), nil
	}
	return protoref.Value{}, fmt.Errorf("unsupported data type %s", dt)
}

// setProtoFieldFromValue sets a field on msg from a protoref.Value, handling
// both scalar and list values. Used by the resolve-context path where values
// are already typed.
func setProtoFieldFromValue(msg protoref.Message, fd protoref.FieldDescriptor, value protoref.Value) error {
	if fd.IsList() {
		dst := msg.Mutable(fd).List()
		src, ok := value.Interface().(protoref.List)
		if !ok {
			return errors.New("expected list value for repeated field")
		}
		for i := range src.Len() {
			dst.Append(src.Get(i))
		}
		return nil
	}
	msg.Set(fd, value)
	return nil
}
