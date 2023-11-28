package variablesvalidation

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type InvalidVariableError struct {
	Message string
}

func (e *InvalidVariableError) Error() string {
	return e.Message
}

type VariablesValidator struct {
	visitor *variablesVisitor
	walker  *astvisitor.Walker
}

func NewVariablesValidator() *VariablesValidator {
	walker := astvisitor.NewWalker(8)
	visitor := &variablesVisitor{
		variables: &astjson.JSON{},
		walker:    &walker,
	}
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
	return &VariablesValidator{
		walker:  &walker,
		visitor: visitor,
	}
}

func (v *VariablesValidator) Validate(operation, definition *ast.Document, variables []byte) error {
	v.visitor.err = nil
	v.visitor.definition = definition
	v.visitor.operation = operation
	err := v.visitor.variables.ParseObject(variables)
	if err != nil {
		return err
	}
	report := &operationreport.Report{}
	v.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return report
	}
	return v.visitor.err
}

type variablesVisitor struct {
	walker                     *astvisitor.Walker
	operation                  *ast.Document
	definition                 *ast.Document
	variables                  *astjson.JSON
	err                        error
	currentVariableName        []byte
	currentVariableJsonNodeRef int
	path                       []pathItem
}

func (v *variablesVisitor) renderPath() string {
	out := &bytes.Buffer{}
	for i, item := range v.path {
		if i > 0 {
			out.WriteString(".")
		}
		out.Write(item.name)
		if item.kind == pathItemKindArray {
			out.WriteString("[")
			out.WriteString(fmt.Sprintf("%d", item.arrayIndex))
			out.WriteString("]")
		}
	}
	return out.String()
}

type pathItemKind int

const (
	pathItemKindObject pathItemKind = iota
	pathItemKindArray
)

type pathItem struct {
	kind       pathItemKind
	name       []byte
	arrayIndex int
}

func (v *variablesVisitor) pushObjectPath(name []byte) {
	v.path = append(v.path, pathItem{
		kind: pathItemKindObject,
		name: name,
	})
}

func (v *variablesVisitor) pushArrayPath(index int) {
	v.path = append(v.path, pathItem{
		kind:       pathItemKindArray,
		arrayIndex: index,
	})
}

func (v *variablesVisitor) popPath() {
	v.path = v.path[:len(v.path)-1]
}

func (v *variablesVisitor) EnterVariableDefinition(ref int) {
	varTypeRef := v.operation.VariableDefinitions[ref].Type
	varName := v.operation.VariableValueNameBytes(v.operation.VariableDefinitions[ref].VariableValue.Ref)
	varTypeName := v.operation.ResolveTypeNameBytes(varTypeRef)
	jsonField := v.variables.GetObjectFieldBytes(v.variables.RootNode, varName)
	if v.operation.TypeIsNonNull(varTypeRef) {
		if jsonField == -1 {
			v.renderVariableRequiredError(varName, varTypeRef)
			return
		}
		if v.variables.Nodes[jsonField].Kind == astjson.NodeKindNull {
			v.renderVariableInvalidNullError(varName, varTypeRef)
			return
		}
	}
	if !v.variables.NodeIsDefined(jsonField) {
		return
	}
	v.path = v.path[:0]
	v.pushObjectPath(varName)
	v.currentVariableName = varName
	v.currentVariableJsonNodeRef = jsonField
	if v.operation.TypeIsList(varTypeRef) {
		if v.variables.Nodes[jsonField].Kind != astjson.NodeKindArray {
			v.renderVariableInvalidTypeError(varTypeName, v.variables.Nodes[jsonField])
			return
		}
		for i, arrayValue := range v.variables.Nodes[jsonField].ArrayValues {
			v.pushArrayPath(i)
			v.traverseNode(arrayValue, varTypeName)
			v.popPath()
			continue
		}
		return
	}
	v.traverseNode(jsonField, varTypeName)
}

func (v *variablesVisitor) renderVariableRequiredError(variableName []byte, typeRef int) {
	out := &bytes.Buffer{}
	err := v.operation.PrintType(typeRef, out)
	if err != nil {
		v.err = err
		return
	}
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" of required type "%s" was not provided.`, string(variableName), out.String()),
	}
}

func (v *variablesVisitor) renderVariableInvalidTypeError(typeName []byte, variablesNode astjson.Node) {
	out := &bytes.Buffer{}
	err := v.variables.PrintNode(variablesNode, out)
	if err != nil {
		v.err = err
		return
	}
	variableContent := out.String()
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" got invalid value %s; Expected type "%s" to be an object.`, string(v.currentVariableName), variableContent, string(typeName)),
	}
}

func (v *variablesVisitor) renderVariableRequiredNotProvidedError(fieldName []byte, typeRef int) {
	out := &bytes.Buffer{}
	err := v.variables.PrintNode(v.variables.Nodes[v.currentVariableJsonNodeRef], out)
	if err != nil {
		v.err = err
		return
	}
	variableContent := out.String()
	out.Reset()
	err = v.definition.PrintType(typeRef, out)
	if err != nil {
		v.err = err
		return
	}
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" got invalid value %s; Field "%s" of required type "%s" was not provided.`, string(v.currentVariableName), variableContent, string(fieldName), out.String()),
	}
}

func (v *variablesVisitor) renderVariableInvalidNestedTypeError(actualJsonNodeRef int, expectedType ast.NodeKind, expectedTypeName []byte) {
	buf := &bytes.Buffer{}
	variableName := string(v.currentVariableName)
	typeName := string(expectedTypeName)
	err := v.variables.PrintNode(v.variables.Nodes[actualJsonNodeRef], buf)
	if err != nil {
		v.err = err
		return
	}
	invalidValue := buf.String()
	var path string
	if len(v.path) > 1 {
		path = fmt.Sprintf(` at "%s"`, v.renderPath())
	}
	switch expectedType {
	case ast.NodeKindScalarTypeDefinition:
		switch typeName {
		case "String":
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; String cannot represent a non string value: %s`, variableName, invalidValue, path, invalidValue),
			}
		case "Int":
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Int cannot represent non-integer value: %s`, variableName, invalidValue, path, invalidValue),
			}
		case "Float":
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Float cannot represent non numeric value: %s`, variableName, invalidValue, path, invalidValue),
			}
		case "Boolean":
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Boolean cannot represent a non boolean value: %s`, variableName, invalidValue, path, invalidValue),
			}
		case "ID":
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; ID cannot represent value: %s`, variableName, invalidValue, path, invalidValue),
			}
		default:
			v.err = &InvalidVariableError{
				Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Expected type "%s" to be a scalar.`, variableName, invalidValue, path, typeName),
			}
		}
	case ast.NodeKindInputObjectTypeDefinition:
		v.err = &InvalidVariableError{
			Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Expected type "%s" to be an object.`, variableName, invalidValue, path, typeName),
		}
	case ast.NodeKindEnumTypeDefinition:
		v.err = &InvalidVariableError{
			Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Enum "%s" cannot represent non-string value: %s.`, variableName, invalidValue, path, typeName, invalidValue),
		}
	}
}

func (v *variablesVisitor) renderVariableFieldNotDefinedError(fieldName []byte, typeName []byte) {
	buf := &bytes.Buffer{}
	variableName := string(v.currentVariableName)
	err := v.variables.PrintNode(v.variables.Nodes[v.currentVariableJsonNodeRef], buf)
	if err != nil {
		v.err = err
		return
	}
	invalidValue := buf.String()
	path := v.renderPath()
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" got invalid value %s at "%s"; Field "%s" is not defined by type "%s".`, variableName, invalidValue, path, string(fieldName), string(typeName)),
	}
}

func (v *variablesVisitor) renderVariableEnumValueDoesNotExistError(typeName []byte, enumValue []byte) {
	buf := &bytes.Buffer{}
	variableName := string(v.currentVariableName)
	err := v.variables.PrintNode(v.variables.Nodes[v.currentVariableJsonNodeRef], buf)
	if err != nil {
		v.err = err
		return
	}
	invalidValue := buf.String()
	var path string
	if len(v.path) > 1 {
		path = fmt.Sprintf(` at "%s"`, v.renderPath())
	}
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" got invalid value %s%s; Value "%s" does not exist in "%s" enum.`, variableName, invalidValue, path, string(enumValue), string(typeName)),
	}
}

func (v *variablesVisitor) renderVariableInvalidNullError(variableName []byte, typeRef int) {
	buf := &bytes.Buffer{}
	err := v.operation.PrintType(typeRef, buf)
	if err != nil {
		v.err = err
		return
	}
	typeName := buf.String()
	v.err = &InvalidVariableError{
		Message: fmt.Sprintf(`Variable "$%s" got invalid value null; Expected non-nullable type "%s" not to be null.`, string(variableName), typeName),
	}
}

func (v *variablesVisitor) traverseNode(jsonNodeRef int, typeName []byte) {
	if v.err != nil {
		return
	}
	fieldTypeDefinitionNode, ok := v.definition.NodeByName(typeName)
	if !ok {
		return
	}
	switch fieldTypeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindObject {
			v.renderVariableInvalidTypeError(typeName, v.variables.Nodes[jsonNodeRef])
			return
		}
		fields := v.definition.NodeInputFieldDefinitions(fieldTypeDefinitionNode)
		for _, field := range fields {
			if v.err != nil {
				return
			}
			fieldName := v.definition.InputValueDefinitionNameBytes(field)
			fieldType := v.definition.InputValueDefinitionType(field)
			fieldVariablesJsonNodeRef := v.variables.GetObjectFieldBytes(jsonNodeRef, fieldName)
			if v.definition.TypeIsNonNull(fieldType) && !v.variables.NodeIsDefined(fieldVariablesJsonNodeRef) {
				v.renderVariableRequiredNotProvidedError(fieldName, fieldType)
				return
			}
			if v.definition.TypeIsList(fieldType) {
				if v.variables.Nodes[fieldVariablesJsonNodeRef].Kind != astjson.NodeKindArray {
					v.renderVariableInvalidNestedTypeError(fieldVariablesJsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
					return
				}
				fieldTypeName := v.definition.ResolveTypeNameBytes(fieldType)
				v.pushObjectPath(fieldName)
				for i, arrayValue := range v.variables.Nodes[fieldVariablesJsonNodeRef].ArrayValues {
					v.pushArrayPath(i)
					v.traverseNode(arrayValue, fieldTypeName)
					v.popPath()
					continue
				}
				v.popPath()
			}
			if !v.variables.NodeIsDefined(fieldVariablesJsonNodeRef) {
				continue
			}
			v.pushObjectPath(fieldName)
			v.traverseNode(fieldVariablesJsonNodeRef, v.definition.ResolveTypeNameBytes(fieldType))
			v.popPath()
		}
		for _, field := range v.variables.Nodes[jsonNodeRef].ObjectFields {
			inputFieldName := v.variables.ObjectFieldKey(field)
			inputValueDefinitionRef := v.definition.InputObjectTypeDefinitionInputValueDefinitionByName(fieldTypeDefinitionNode.Ref, inputFieldName)
			if inputValueDefinitionRef == -1 {
				v.renderVariableFieldNotDefinedError(inputFieldName, typeName)
				return
			}
		}
	case ast.NodeKindScalarTypeDefinition:
		switch unsafebytes.BytesToString(typeName) {
		case "String":
			if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindString {
				v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
				return
			}
		case "Int":
			if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindNumber {
				v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
				return
			}
		case "Float":
			if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindNumber {
				v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
				return
			}
		case "Boolean":
			if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindBoolean {
				v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
				return
			}
		case "ID":
			if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindString && v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindNumber {
				v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
				return
			}
		}
	case ast.NodeKindEnumTypeDefinition:
		if v.variables.Nodes[jsonNodeRef].Kind != astjson.NodeKindString {
			v.renderVariableInvalidNestedTypeError(jsonNodeRef, fieldTypeDefinitionNode.Kind, typeName)
			return
		}
		value := v.variables.Nodes[jsonNodeRef].ValueBytes(v.variables)
		if !v.definition.EnumTypeDefinitionContainsEnumValue(fieldTypeDefinitionNode.Ref, value) {
			v.renderVariableEnumValueDoesNotExistError(typeName, value)
			return
		}
	}
}
