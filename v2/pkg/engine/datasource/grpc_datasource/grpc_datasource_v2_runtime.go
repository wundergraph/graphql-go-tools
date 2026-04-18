package grpcdatasource

import (
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// V2 runtime — executes a pre-compiled v2Program against per-request state.
//
// Design: every hot-path decision (which field descriptor, which backend,
// which scalar kind) was resolved at compile time and lives on the IR nodes.
// Runtime just walks.

// buildRequest populates an outgoing protobuf message from the compiled
// request program and the request variables.
func (p *v2RequestProgram) buildRequest(data gjson.Result, schema *v2SchemaRuntime, compiler *RPCCompiler) (protoref.Message, error) {
	msg := p.message.newMessage()
	if err := p.populateFields(msg, data, schema, compiler); err != nil {
		return nil, err
	}
	return msg, nil
}

// buildResolveRequest is buildRequest + attaches a context list built from
// the dependency fetch's output. Returns skip=true when the dependency had
// no rows — matches the V1 behavior for empty resolver batches.
func (p *v2RequestProgram) buildResolveRequest(data gjson.Result, dependency protoref.Message, schema *v2SchemaRuntime, compiler *RPCCompiler) (protoref.Message, bool, error) {
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
					if !field.matchesMemberType(element) {
						continue
					}
					childMsg, err := field.child.buildRequest(element, schema, compiler)
					if err != nil {
						return err
					}
					list.Append(protoref.ValueOfMessage(childMsg))
					continue
				}
				list.Append(compiler.setValueForKind(field.runtime.dataType, element))
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
			childMsg, err := field.child.buildRequest(fieldData, schema, compiler)
			if err != nil {
				return err
			}
			message.Set(fd, protoref.ValueOfMessage(childMsg))
			continue
		}

		if field.staticValue != "" {
			message.Set(fd, compiler.setValueForKind(field.runtime.dataType, gjson.Parse(field.staticValue)))
			continue
		}

		fieldData := data.Get(field.jsonPath)
		if isNullValue(fieldData) {
			if field.optional {
				continue
			}
			return fmt.Errorf("field %s is required but has no value", field.jsonPath)
		}
		message.Set(fd, compiler.setValueForKind(field.runtime.dataType, fieldData))
	}
	return nil
}

// extractRows pulls a list of protobuf row messages from a dependency output
// by walking each compiled field's path in parallel. All paths must produce
// the same number of values — each row gets one value per field.
func (p *v2ContextProgram) extractRows(message protoref.Message) ([]protoref.Message, error) {
	var rows []protoref.Message
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

// attach merges a fetch's response into the growing frame rooted at `root`.
// Standard fetches contribute their top-level fields directly; resolve-kind
// fetches walk responsePath and merge per-row.
func (p *v2ResponseProgram) attach(frame *v2ResponseFrameBuilder, root int, data protoref.Message, kind CallKind, path ast.Path, mapping *GRPCMapping) error {
	if data == nil || !data.IsValid() {
		return nil
	}
	switch kind {
	case CallKindResolve, CallKindRequired:
		return p.attachResolve(frame, root, data, path, mapping)
	default:
		return p.applyObject(frame, root, data, mapping)
	}
}

func (p *v2ResponseProgram) applyObject(frame *v2ResponseFrameBuilder, root int, data protoref.Message, mapping *GRPCMapping) error {
	for i := range p.fields {
		field := &p.fields[i]
		value, err := field.materialize(frame, data, mapping)
		if err != nil {
			return err
		}
		frame.setObjectField(root, field.name, value)
	}
	return nil
}

func (p *v2ResponseProgram) attachResolve(frame *v2ResponseFrameBuilder, root int, data protoref.Message, path ast.Path, mapping *GRPCMapping) error {
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
		value, err := attachField.materialize(frame, list.Get(i).Message(), mapping)
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

func (f *v2ResponseFieldProgram) materialize(frame *v2ResponseFrameBuilder, data protoref.Message, mapping *GRPCMapping) (int, error) {
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
				childValue, err := f.child.objectValue(frame, list.Get(i).Message(), mapping)
				if err != nil {
					return 0, err
				}
				frame.appendArrayItem(arr, childValue)
				continue
			}
			value, err := scalarFrameValue(frame, list.Get(i), fd, mapping)
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
		return f.child.objectValue(frame, msg, mapping)
	}
	return scalarFrameValue(frame, data.Get(fd), fd, mapping)
}

func (p *v2ResponseProgram) objectValue(frame *v2ResponseFrameBuilder, data protoref.Message, mapping *GRPCMapping) (int, error) {
	if data == nil || !data.IsValid() {
		return frame.newNull(), nil
	}
	root := frame.newObject()
	if err := p.applyObject(frame, root, data, mapping); err != nil {
		return 0, err
	}
	return root, nil
}

// scalarFrameValue writes a proto scalar into the frame using the pre-
// formatted string representation strconv produces. Number formatting cost is
// incurred here once per value; the serializer later just appends the string
// unchanged.
func scalarFrameValue(frame *v2ResponseFrameBuilder, data protoref.Value, fd protoref.FieldDescriptor, mapping *GRPCMapping) (int, error) {
	if !data.IsValid() {
		return frame.newNull(), nil
	}
	switch fd.Kind() {
	case protoref.BoolKind:
		return frame.newBool(data.Bool()), nil
	case protoref.StringKind:
		return frame.newString(data.String()), nil
	case protoref.Int32Kind, protoref.Int64Kind:
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
		graphqlValue, ok := mapping.FindEnumValueMapping(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			return frame.newNull(), nil
		}
		return frame.newString(graphqlValue), nil
	default:
		return frame.newNull(), fmt.Errorf("unsupported scalar kind %s", fd.Kind())
	}
}
