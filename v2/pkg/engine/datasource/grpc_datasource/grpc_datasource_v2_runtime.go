package grpcdatasource

import (
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func (p *v2RequestProgram) build(data gjson.Result, schema *v2SchemaRuntime, compiler *RPCCompiler) (protoref.Message, error) {
	msg := p.message.newMessage()

	if err := p.populateFields(msg, data, schema, compiler); err != nil {
		return nil, err
	}

	return msg, nil
}

func (p *v2RequestProgram) buildInput(data gjson.Result, schema *v2SchemaRuntime, compiler *RPCCompiler) (any, error) {
	if p.wire != nil {
		wire, err := p.wire.execute(nil, data)
		if err != nil {
			return nil, err
		}
		return &v2PreMarshaledInput{wire: wire}, nil
	}
	return p.build(data, schema, compiler)
}

func (p *v2RequestProgram) buildWithDependency(data gjson.Result, dependency protoref.Message, schema *v2SchemaRuntime, compiler *RPCCompiler) (protoref.Message, bool, error) {
	msg := p.message.newMessage()

	if err := p.populateFields(msg, data, schema, compiler); err != nil {
		return nil, false, err
	}

	if p.context == nil {
		return msg, false, nil
	}
	if dependency == nil || !dependency.IsValid() {
		return nil, true, nil
	}

	rows, err := p.context.extractRows(dependency)
	if err != nil {
		return nil, false, err
	}
	if len(rows) == 0 {
		return nil, true, nil
	}

	contextList := msg.Mutable(p.context.runtime.descriptorFor(msg)).List()
	for _, row := range rows {
		contextList.Append(protoref.ValueOfMessage(row))
	}

	return msg, false, nil
}

func (p *v2RequestProgram) populateFields(message protoref.Message, data gjson.Result, schema *v2SchemaRuntime, compiler *RPCCompiler) error {
	for i := range p.fields {
		field := &p.fields[i]
		fd := field.runtime.descriptorFor(message)

		if field.repeated {
			elements := data.Get(field.jsonPath).Array()
			if len(elements) == 0 {
				continue
			}

			list := message.Mutable(fd).List()
			for _, element := range elements {
				if field.child != nil {
					childMsg, err := field.child.build(element, schema, compiler)
					if err != nil {
						return err
					}
					list.Append(protoref.ValueOfMessage(childMsg))
					continue
				}

				value, err := field.inputValue(element, compiler)
				if err != nil {
					return err
				}
				list.Append(value)
			}
			continue
		}

		if field.child != nil {
			fieldData := data
			if field.jsonPath != "" {
				fieldData = data.Get(field.jsonPath)
			}
			if isNullValue(fieldData) {
				if field.optional {
					continue
				}
				return fmt.Errorf("field %s is required but has no value", field.jsonPath)
			}

			childMsg, err := field.child.build(fieldData, schema, compiler)
			if err != nil {
				return err
			}
			message.Set(fd, protoref.ValueOfMessage(childMsg))
			continue
		}

		if field.staticValue != "" {
			value, err := field.inputValue(gjson.Parse(field.staticValue), compiler)
			if err != nil {
				return err
			}
			message.Set(fd, value)
			continue
		}

		fieldData := data.Get(field.jsonPath)
		if isNullValue(fieldData) {
			if field.optional {
				continue
			}
			return fmt.Errorf("field %s is required but has no value", field.jsonPath)
		}

		value, err := field.inputValue(fieldData, compiler)
		if err != nil {
			return err
		}
		message.Set(fd, value)
	}

	return nil
}

func (f *v2RequestFieldProgram) inputValue(data gjson.Result, compiler *RPCCompiler) (protoref.Value, error) {
	if f.runtime.dataType == DataTypeEnum {
		return compiler.getEnumValue(f.enumName, data)
	}
	return compiler.setValueForKind(f.runtime.dataType, data), nil
}

func (p *v2ContextProgram) extractRows(message protoref.Message) ([]protoref.Message, error) {
	rows := make([]protoref.Message, 0)

	for i := range p.fields {
		field := &p.fields[i]
		values := field.path.extract(message)
		if len(values) == 0 {
			return nil, nil
		}

		if len(rows) == 0 {
			rows = make([]protoref.Message, len(values))
			for index := range values {
				rows[index] = p.message.newMessage()
			}
		}

		if len(values) != len(rows) {
			return nil, fmt.Errorf("resolve context field %s produced %d values, expected %d", field.runtime.name, len(values), len(rows))
		}

		for index, value := range values {
			rows[index].Set(field.runtime.descriptorFor(rows[index]), value)
		}
	}

	return rows, nil
}

func (p v2ResolvePathProgram) extract(message protoref.Message) []protoref.Value {
	if !message.IsValid() {
		return nil
	}

	return p.extractFromMessage(message, 0)
}

func (p v2ResolvePathProgram) extractFromMessage(message protoref.Message, stepIndex int) []protoref.Value {
	if !message.IsValid() || stepIndex >= len(p.steps) {
		return nil
	}

	step := p.steps[stepIndex]
	fieldValue := message.Get(step.runtime.descriptorFor(message))
	if !fieldValue.IsValid() {
		return nil
	}

	if step.runtime.repeated {
		list := fieldValue.List()
		if !list.IsValid() || list.Len() == 0 {
			return nil
		}

		result := make([]protoref.Value, 0, list.Len())
		for i := 0; i < list.Len(); i++ {
			item := list.Get(i)
			if step.runtime.isMessage && stepIndex < len(p.steps)-1 {
				result = append(result, p.extractFromMessage(item.Message(), stepIndex+1)...)
				continue
			}
			result = append(result, item)
		}
		return result
	}

	if step.runtime.isMessage {
		if stepIndex == len(p.steps)-1 {
			return []protoref.Value{protoref.ValueOfMessage(fieldValue.Message())}
		}
		return p.extractFromMessage(fieldValue.Message(), stepIndex+1)
	}

	return []protoref.Value{fieldValue}
}

func (p *v2ResponseProgram) attach(builder *jsonBuilder, frame *v2ResponseFrameBuilder, root int, data protoref.Message, kind CallKind, path ast.Path) error {
	if data == nil || !data.IsValid() {
		return nil
	}

	switch kind {
	case CallKindResolve, CallKindRequired:
		return p.attachResolve(builder, frame, root, data, path)
	default:
		return p.applyObject(builder, frame, root, data)
	}
}

func (p *v2ResponseProgram) validateFederatedOutput(builder *jsonBuilder, data protoref.Message) error {
	if len(builder.indexMap) == 0 || data == nil || !data.IsValid() {
		return nil
	}

	entitiesField := p.fieldByName("_entities")
	if entitiesField == nil {
		return nil
	}

	fd := entitiesField.runtime.descriptorFor(data)
	if !fd.IsList() || entitiesField.child == nil {
		return fmt.Errorf("federated response field %s must be a repeated message", entitiesField.name)
	}

	typenameField := entitiesField.child.fieldByName("__typename")
	if typenameField == nil {
		return fmt.Errorf("federated response field %s is missing __typename", entitiesField.name)
	}

	entities := data.Get(fd).List()
	entityCountPerType := make(map[string]int)
	for i := 0; i < entities.Len(); i++ {
		entity := entities.Get(i).Message()
		if !entity.IsValid() {
			continue
		}
		typeName := typenameField.staticValue
		if typeName == "" {
			if typenameField.runtime == nil {
				return fmt.Errorf("federated response field %s is missing a runtime or static typename", typenameField.name)
			}
			typeName = entity.Get(typenameField.runtime.descriptorFor(entity)).String()
		}
		if typeName == "" {
			continue
		}
		entityCountPerType[typeName]++
	}

	for typeName, count := range entityCountPerType {
		expected, found := builder.indexMap[typeName]
		if !found {
			return fmt.Errorf("entity type %s received in the subgraph response, but was not expected", typeName)
		}
		if len(expected) != count {
			return fmt.Errorf("entity type %s received %d entities in the subgraph response, but %d are expected", typeName, count, len(expected))
		}
	}

	return nil
}

func (p *v2ResponseProgram) applyObject(builder *jsonBuilder, frame *v2ResponseFrameBuilder, root int, data protoref.Message) error {
	for i := range p.fields {
		field := &p.fields[i]
		value, err := field.materialize(builder, frame, data)
		if err != nil {
			return err
		}
		frame.setObjectField(root, field.name, value)
	}
	return nil
}

func (p *v2ResponseProgram) attachResolve(builder *jsonBuilder, frame *v2ResponseFrameBuilder, root int, data protoref.Message, path ast.Path) error {
	if len(path) == 0 {
		return fmt.Errorf("resolve response path is empty")
	}
	if len(p.fields) != 1 {
		return fmt.Errorf("resolve response requires exactly one top-level field, got %d", len(p.fields))
	}

	resultField := &p.fields[0]
	fd := resultField.runtime.descriptorFor(data)
	if !fd.IsList() || resultField.child == nil {
		return fmt.Errorf("resolve response field %s must be a repeated message", resultField.name)
	}

	list := data.Get(fd).List()
	if !list.IsValid() || list.Len() == 0 {
		return nil
	}

	searchPath := path[:len(path)-1]
	elementName := path[len(path)-1].FieldName.String()

	targets, err := p.resolveAttachTargets(frame, root, searchPath)
	if err != nil {
		return err
	}
	if len(targets) != list.Len() {
		return fmt.Errorf("length of values doesn't match the length of the result array, expected %d, got %d", len(targets), list.Len())
	}

	attachField := resultField.child.fieldByName(elementName)
	if attachField == nil {
		return fmt.Errorf("resolve result field %s not found", elementName)
	}

	for i := 0; i < list.Len(); i++ {
		value, err := attachField.materialize(builder, frame, list.Get(i).Message())
		if err != nil {
			return err
		}
		frame.setObjectField(targets[i], elementName, value)
	}

	return nil
}

func (p *v2ResponseProgram) resolveAttachTargets(frame *v2ResponseFrameBuilder, root int, path ast.Path) ([]int, error) {
	if path.Len() == 0 {
		return []int{root}, nil
	}

	next, ok := frame.getObjectField(root, path[0].FieldName.String())
	if !ok {
		return nil, fmt.Errorf("response path %s not found", path.String())
	}
	return frame.flatten(next, path[1:])
}

func (p *v2ResponseProgram) fieldByName(name string) *v2ResponseFieldProgram {
	for i := range p.fields {
		if p.fields[i].name == name {
			return &p.fields[i]
		}
	}
	return nil
}

func (f *v2ResponseFieldProgram) materialize(builder *jsonBuilder, frame *v2ResponseFrameBuilder, data protoref.Message) (int, error) {
	if f.staticValue != "" {
		return frame.newString(f.staticValue), nil
	}

	fd := f.runtime.descriptorFor(data)
	if fd.IsList() {
		arr := frame.newArray()
		list := data.Get(fd).List()
		if !list.IsValid() {
			return arr, nil
		}

		for i := 0; i < list.Len(); i++ {
			if f.child != nil {
				childValue, err := f.child.objectValue(builder, frame, list.Get(i).Message())
				if err != nil {
					return 0, err
				}
				frame.appendArrayItem(arr, childValue)
				continue
			}

			value, err := scalarFrameValue(builder, frame, list.Get(i), fd)
			if err != nil {
				return 0, err
			}
			frame.appendArrayItem(arr, value)
		}

		return arr, nil
	}

	if f.child != nil {
		msg := data.Get(fd).Message()
		if !msg.IsValid() {
			return frame.newNull(), nil
		}
		return f.child.objectValue(builder, frame, msg)
	}

	return scalarFrameValue(builder, frame, data.Get(fd), fd)
}

func (p *v2ResponseProgram) objectValue(builder *jsonBuilder, frame *v2ResponseFrameBuilder, data protoref.Message) (int, error) {
	if data == nil || !data.IsValid() {
		return frame.newNull(), nil
	}
	if p.oneOfType != OneOfTypeNone {
		return p.oneOfObjectValue(builder, frame, data)
	}

	root := frame.newObject()
	if err := p.applyObject(builder, frame, root, data); err != nil {
		return 0, err
	}
	return root, nil
}

func (p *v2ResponseProgram) oneOfObjectValue(builder *jsonBuilder, frame *v2ResponseFrameBuilder, data protoref.Message) (int, error) {
	root := frame.newObject()
	if err := p.applyObject(builder, frame, root, data); err != nil {
		return 0, err
	}

	oneofDesc := data.Descriptor().Oneofs().ByName(protoref.Name(p.oneOfType.FieldName()))
	if oneofDesc == nil {
		return 0, fmt.Errorf("oneof %s not found on message %s", p.oneOfType.FieldName(), data.Descriptor().FullName())
	}

	activeField := data.WhichOneof(oneofDesc)
	if activeField == nil {
		return root, nil
	}

	activeMessage := data.Get(activeField).Message()
	if !activeMessage.IsValid() {
		return root, nil
	}

	fragmentProgram, ok := p.fragments[string(activeField.Message().Name())]
	if !ok {
		return root, nil
	}
	if err := fragmentProgram.applyObject(builder, frame, root, activeMessage); err != nil {
		return 0, err
	}

	return root, nil
}

func (p *v2ResponseProgram) write(builder *jsonBuilder, frame *v2ResponseFrameBuilder, data protoref.Message) (int, error) {
	return p.objectValue(builder, frame, data)
}

func scalarFrameValue(builder *jsonBuilder, frame *v2ResponseFrameBuilder, data protoref.Value, fd protoref.FieldDescriptor) (int, error) {
	if !data.IsValid() {
		return frame.newNull(), nil
	}

	switch fd.Kind() {
	case protoref.BoolKind:
		if data.Bool() {
			return frame.newBool(true), nil
		}
		return frame.newBool(false), nil
	case protoref.StringKind:
		return frame.newString(data.String()), nil
	case protoref.Int32Kind:
		return frame.newNumber(strconv.FormatInt(data.Int(), 10)), nil
	case protoref.Int64Kind:
		return frame.newNumber(strconv.FormatInt(data.Int(), 10)), nil
	case protoref.Uint32Kind, protoref.Uint64Kind:
		return frame.newNumber(strconv.FormatUint(data.Uint(), 10)), nil
	case protoref.FloatKind, protoref.DoubleKind:
		return frame.newNumber(strconv.FormatFloat(data.Float(), 'g', -1, 64)), nil
	case protoref.BytesKind:
		return frame.newString(string(data.Bytes())), nil
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumValueDesc := enumDesc.Values().ByNumber(data.Enum())
		if enumValueDesc == nil {
			return frame.newNull(), nil
		}
		graphqlValue, ok := builder.mapping.FindEnumValueMapping(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			return frame.newNull(), nil
		}
		return frame.newString(graphqlValue), nil
	default:
		return frame.newNull(), fmt.Errorf("unsupported scalar kind %s", fd.Kind())
	}
}
