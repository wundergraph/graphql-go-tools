package variablesvalidation

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/apollocompatibility"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/errorcodes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/federation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type InvalidVariableError struct {
	ExtensionCode string
	Message       string
}

func (e *InvalidVariableError) Error() string {
	return e.Message
}

func (v *variablesVisitor) invalidValueIfAllowed(variableContent string) string {
	if v.opts.DisableExposingVariablesContent {
		return ""
	}

	return fmt.Sprintf(": %s", variableContent)
}

func (v *variablesVisitor) invalidEnumValueIfAllowed(variableContent string) string {
	if v.opts.DisableExposingVariablesContent {
		return ""
	}

	return fmt.Sprintf("\"%s\" ", variableContent)
}

func (v *variablesVisitor) invalidValueMessage(variableName, variableContent string) string {
	if v.opts.DisableExposingVariablesContent {
		return fmt.Sprintf(`Variable "$%s" got invalid value`, variableName)
	}

	return fmt.Sprintf(`Variable "$%s" got invalid value %s`, variableName, variableContent)
}

func (v *variablesVisitor) newInvalidVariableError(message string) *InvalidVariableError {
	if v.opts.ApolloRouterCompatibilityFlags.ReplaceInvalidVarError {
		return &InvalidVariableError{
			Message:       fmt.Sprintf("invalid type for variable: '%s'", v.currentVariableName),
			ExtensionCode: errorcodes.ValidationInvalidTypeVariable,
		}
	}

	err := &InvalidVariableError{
		Message: message,
	}

	if v.opts.ApolloCompatibilityFlags.ReplaceInvalidVarError {
		err.ExtensionCode = errorcodes.BadUserInput
	}
	return err
}

type VariablesValidator struct {
	visitor *variablesVisitor
	walker  *astvisitor.Walker
}

type VariablesValidatorOptions struct {
	ApolloCompatibilityFlags        apollocompatibility.Flags
	ApolloRouterCompatibilityFlags  apollocompatibility.ApolloRouterFlags
	DisableExposingVariablesContent bool
}

func NewVariablesValidator(options VariablesValidatorOptions) *VariablesValidator {
	walker := astvisitor.NewWalker(8)
	visitor := &variablesVisitor{
		walker: &walker,
		opts:   options,
	}
	walker.RegisterEnterVariableDefinitionVisitor(visitor)
	return &VariablesValidator{
		walker:  &walker,
		visitor: visitor,
	}
}

func (v *VariablesValidator) ValidateWithRemap(operation, definition *ast.Document, variables []byte, variablesMap map[string]string) error {
	v.visitor.variablesMap = variablesMap
	return v.Validate(operation, definition, variables)
}

func (v *VariablesValidator) Validate(operation, definition *ast.Document, variables []byte) error {
	v.visitor.definition = definition
	v.visitor.operation = operation
	v.visitor.variables, v.visitor.err = astjson.ParseBytes(variables)
	if v.visitor.err != nil {
		return v.visitor.err
	}
	report := &operationreport.Report{}
	v.walker.Walk(operation, definition, report)
	if report.HasErrors() {
		return report
	}
	return v.visitor.err
}

type variablesVisitor struct {
	walker               *astvisitor.Walker
	operation            *ast.Document
	definition           *ast.Document
	variables            *astjson.Value
	err                  error
	currentVariableName  []byte
	currentVariableValue *astjson.Value
	path                 []pathItem
	opts                 VariablesValidatorOptions
	variablesMap         map[string]string
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
			fmt.Fprint(out, item.arrayIndex)
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
	operationVariableNameBytes := v.operation.VariableValueNameBytes(v.operation.VariableDefinitions[ref].VariableValue.Ref)
	operationVariableName := unsafebytes.BytesToString(operationVariableNameBytes)

	var (
		mappedVariableName string
		isMapped           bool
	)
	if v.variablesMap != nil {
		mappedVariableName, isMapped = v.variablesMap[operationVariableName]
	}

	if isMapped {
		v.currentVariableName = []byte(mappedVariableName)
		v.currentVariableValue = v.variables.Get(mappedVariableName)
	} else {
		v.currentVariableName = operationVariableNameBytes
		v.currentVariableValue = v.variables.Get(operationVariableName)
	}

	v.path = v.path[:0]
	v.pushObjectPath(v.currentVariableName)

	v.traverseOperationType(v.currentVariableValue, varTypeRef)
}

func (v *variablesVisitor) traverseOperationType(jsonValue *astjson.Value, operationTypeRef int) {
	varTypeName := v.operation.ResolveTypeNameBytes(operationTypeRef)
	if v.operation.TypeIsNonNull(operationTypeRef) {
		if jsonValue == nil {
			v.renderVariableRequiredError(v.currentVariableName, operationTypeRef)
			return
		}

		if jsonValue.Type() == astjson.TypeNull && varTypeName.String() != "Upload" {
			v.renderVariableInvalidNullError(v.currentVariableName, operationTypeRef)
			return
		}

		v.traverseOperationType(jsonValue, v.operation.Types[operationTypeRef].OfType)
		return
	}

	if jsonValue == nil || jsonValue.Type() == astjson.TypeNull {
		return
	}

	if v.operation.TypeIsList(operationTypeRef) {
		if jsonValue.Type() != astjson.TypeArray {
			v.renderVariableInvalidObjectTypeError(varTypeName, jsonValue)
			return
		}
		values := jsonValue.GetArray()
		for i, arrayValue := range values {
			v.pushArrayPath(i)
			v.traverseOperationType(arrayValue, v.operation.Types[operationTypeRef].OfType)
			v.popPath()
			continue
		}
		return
	}

	v.traverseNamedTypeNode(jsonValue, varTypeName)
}

func (v *variablesVisitor) renderVariableRequiredError(variableName []byte, typeRef int) {
	out := &bytes.Buffer{}
	err := v.operation.PrintType(typeRef, out)
	if err != nil {
		v.err = err
		return
	}
	v.err = v.newInvalidVariableError(fmt.Sprintf(`Variable "$%s" of required type "%s" was not provided.`, string(variableName), out.String()))
}

func (v *variablesVisitor) renderVariableInvalidObjectTypeError(typeName []byte, variablesNode *astjson.Value) {
	variableContent := string(variablesNode.MarshalTo(nil))
	v.err = v.newInvalidVariableError(fmt.Sprintf(`%s; Expected type "%s" to be an object.`, v.invalidValueMessage(string(v.currentVariableName), variableContent), string(typeName)))
}

func (v *variablesVisitor) renderVariableRequiredNotProvidedError(fieldName []byte, typeRef int) {
	variableContent := string(v.currentVariableValue.MarshalTo(nil))
	out := &bytes.Buffer{}
	err := v.definition.PrintType(typeRef, out)
	if err != nil {
		v.err = err
		return
	}
	v.err = v.newInvalidVariableError(fmt.Sprintf(`%s; Field "%s" of required type "%s" was not provided.`, v.invalidValueMessage(string(v.currentVariableName), variableContent), string(fieldName), out.String()))
}

func (v *variablesVisitor) renderVariableInvalidNestedTypeError(jsonValue *astjson.Value, expectedType ast.NodeKind, expectedTypeName []byte, expectedList bool) {
	variableName := string(v.currentVariableName)
	typeName := string(expectedTypeName)
	invalidValue := string(jsonValue.MarshalTo(nil))
	var path string
	if len(v.path) > 1 {
		path = fmt.Sprintf(` at "%s"`, v.renderPath())
	}
	switch expectedType {
	case ast.NodeKindScalarTypeDefinition:
		switch typeName {
		case "String":
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; String cannot represent a non string value%s`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidValueIfAllowed(invalidValue)))
		case "Int":
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Int cannot represent non-integer value%s`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidValueIfAllowed(invalidValue)))
		case "Float":
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Float cannot represent non numeric value%s`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidValueIfAllowed(invalidValue)))
		case "Boolean":
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Boolean cannot represent a non boolean value%s`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidValueIfAllowed(invalidValue)))
		case "ID":
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; ID cannot represent a non-string and non-integer value%s`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidValueIfAllowed(invalidValue)))
		default:
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Expected type "%s" to be a scalar.`, v.invalidValueMessage(variableName, invalidValue), path, typeName))
		}
	case ast.NodeKindInputObjectTypeDefinition:
		if expectedList {
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Got input type "%s", want: "[%s]"`, v.invalidValueMessage(variableName, invalidValue), path, typeName, typeName))
		} else {
			v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Expected type "%s" to be an input object.`, v.invalidValueMessage(variableName, invalidValue), path, typeName))
		}
	case ast.NodeKindEnumTypeDefinition:
		v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Enum "%s" cannot represent non-string value%s.`, v.invalidValueMessage(variableName, invalidValue), path, typeName, v.invalidValueIfAllowed(invalidValue)))
	}
}

func (v *variablesVisitor) renderVariableFieldNotDefinedError(fieldName []byte, typeName []byte) {
	variableName := string(v.currentVariableName)
	invalidValue := string(v.currentVariableValue.MarshalTo(nil))
	path := v.renderPath()
	v.err = v.newInvalidVariableError(fmt.Sprintf(`%s at "%s"; Field "%s" is not defined by type "%s".`, v.invalidValueMessage(variableName, invalidValue), path, string(fieldName), string(typeName)))
}

func (v *variablesVisitor) renderVariableEnumValueDoesNotExistError(typeName []byte, enumValue []byte) {
	variableName := string(v.currentVariableName)
	invalidValue := string(v.currentVariableValue.MarshalTo(nil))
	var path string
	if len(v.path) > 1 {
		path = fmt.Sprintf(` at "%s"`, v.renderPath())
	}
	v.err = v.newInvalidVariableError(fmt.Sprintf(`%s%s; Value %sdoes not exist in "%s" enum.`, v.invalidValueMessage(variableName, invalidValue), path, v.invalidEnumValueIfAllowed(string(enumValue)), string(typeName)))
}

func (v *variablesVisitor) renderVariableInvalidNullError(variableName []byte, typeRef int) {
	buf := &bytes.Buffer{}
	err := v.operation.PrintType(typeRef, buf)
	if err != nil {
		v.err = err
		return
	}
	typeName := buf.String()
	v.err = v.newInvalidVariableError(fmt.Sprintf(`Variable "$%s" got invalid value null; Expected non-nullable type "%s" not to be null.`, string(variableName), typeName))
}

func (v *variablesVisitor) traverseFieldDefinitionType(fieldTypeDefinitionNodeKind ast.NodeKind, fieldName ast.ByteSlice, jsonValue *astjson.Value, typeRef, inputFieldRef int) {
	if v.definition.TypeIsNonNull(typeRef) {
		if jsonValue == nil || jsonValue.Type() == astjson.TypeNull {

			if bytes.Equal(v.definition.TypeNameBytes(v.definition.Types[typeRef].OfType), []byte("Upload")) {
				return
			}

			// An undefined required input field is valid if it has a default value
			if v.definition.InputValueDefinitionHasDefaultValue(inputFieldRef) {
				return
			}
			v.renderVariableRequiredNotProvidedError(fieldName, typeRef)
		}

		v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, jsonValue, v.definition.Types[typeRef].OfType, inputFieldRef)

		return
	}

	if jsonValue == nil || jsonValue.Type() == astjson.TypeNull {
		return
	}

	typeName := v.definition.ResolveTypeNameBytes(typeRef)

	if v.definition.TypeIsList(typeRef) {
		if jsonValue.Type() != astjson.TypeArray {
			v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNodeKind, typeName, true)
			return
		}
		if len(jsonValue.GetArray()) == 0 {
			return
		}

		for i, arrayValue := range jsonValue.GetArray() {
			v.pushArrayPath(i)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, arrayValue, v.definition.Types[typeRef].OfType, inputFieldRef)
			v.popPath()
			continue
		}
		return
	}

	v.traverseNamedTypeNode(jsonValue, v.definition.ResolveTypeNameBytes(typeRef))
}

func (v *variablesVisitor) violatesOneOfConstraint(inputObjectDefRef int, jsonValue *astjson.Value, typeName []byte) bool {
	def := v.definition.InputObjectTypeDefinitions[inputObjectDefRef]

	// Check if the input object type has @oneOf directive
	if !def.HasDirectives {
		return false
	}
	hasOneOfDirective := def.Directives.HasDirectiveByName(v.definition, "oneOf")
	if !hasOneOfDirective {
		return false
	}

	obj := jsonValue.GetObject()
	totalFieldCount := obj.Len()

	// Prioritize the count error
	if totalFieldCount != 1 {
		variableContent := string(jsonValue.MarshalTo(nil))
		var path string
		if len(v.path) > 1 {
			path = fmt.Sprintf(` at "%s"`, v.renderPath())
		}
		message := fmt.Sprintf(`%s%s; OneOf input object "%s" must have exactly one field provided, but %d fields were provided.`,
			v.invalidValueMessage(string(v.currentVariableName), variableContent),
			path,
			string(typeName),
			totalFieldCount)
		v.err = v.newInvalidVariableError(message)
		return true
	}

	// Check if the single field has a null value
	var nullFieldName []byte
	obj.Visit(func(key []byte, val *astjson.Value) {
		if val.Type() == astjson.TypeNull {
			nullFieldName = key
		}
	})

	if nullFieldName == nil {
		// We have exactly one field, and it's non-null
		return false
	}

	variableContent := string(jsonValue.MarshalTo(nil))
	var path string
	if len(v.path) > 1 {
		path = fmt.Sprintf(` at "%s"`, v.renderPath())
	}
	v.err = v.newInvalidVariableError(
		fmt.Sprintf(`%s%s; OneOf input object "%s" field "%s" value must be non-null.`,
			v.invalidValueMessage(string(v.currentVariableName), variableContent),
			path,
			string(typeName),
			string(nullFieldName)))
	return true
}

func (v *variablesVisitor) traverseNamedTypeNode(jsonValue *astjson.Value, typeName []byte) {
	if v.err != nil {
		return
	}
	fieldTypeDefinitionNode, ok := v.definition.NodeByName(typeName)
	if !ok {
		return
	}
	switch fieldTypeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		if jsonValue.Type() != astjson.TypeObject {
			v.renderVariableInvalidObjectTypeError(typeName, jsonValue)
			return
		}
		inputFieldRefs := v.definition.NodeInputFieldDefinitions(fieldTypeDefinitionNode)
		for _, inputFieldRef := range inputFieldRefs {
			if v.err != nil {
				return
			}
			fieldName := v.definition.InputValueDefinitionNameBytes(inputFieldRef)
			fieldTypeRef := v.definition.InputValueDefinitionType(inputFieldRef)
			objectFieldValue := jsonValue.Get(unsafebytes.BytesToString(fieldName))

			v.pushObjectPath(fieldName)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNode.Kind, fieldName, objectFieldValue, fieldTypeRef, inputFieldRef)
			v.popPath()
		}
		// validate that all input fields present in object are defined in the input object definition
		obj := jsonValue.GetObject()
		keys := make([][]byte, obj.Len())
		i := 0
		obj.Visit(func(key []byte, v *astjson.Value) {
			keys[i] = key
			i++
		})
		for i := range keys {
			inputFieldName := keys[i]
			inputValueDefinitionRef := v.definition.InputObjectTypeDefinitionInputValueDefinitionByName(fieldTypeDefinitionNode.Ref, inputFieldName)
			if inputValueDefinitionRef == -1 {
				v.renderVariableFieldNotDefinedError(inputFieldName, typeName)
				return
			}
		}
		if v.violatesOneOfConstraint(fieldTypeDefinitionNode.Ref, jsonValue, typeName) {
			return // Error already reported
		}
	case ast.NodeKindScalarTypeDefinition:
		switch unsafebytes.BytesToString(typeName) {
		case "String":
			if jsonValue.Type() != astjson.TypeString {
				v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
				return
			}
		case "Int":
			if jsonValue.Type() != astjson.TypeNumber {
				v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
				return
			}
		case "Float":
			if jsonValue.Type() != astjson.TypeNumber {
				v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
				return
			}
		case "Boolean":
			if jsonValue.Type() != astjson.TypeTrue && jsonValue.Type() != astjson.TypeFalse {
				v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
				return
			}
		case "ID":
			if jsonValue.Type() != astjson.TypeString && jsonValue.Type() != astjson.TypeNumber {
				v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
				return
			}
		}
	case ast.NodeKindEnumTypeDefinition:
		if jsonValue.Type() != astjson.TypeString {
			v.renderVariableInvalidNestedTypeError(jsonValue, fieldTypeDefinitionNode.Kind, typeName, false)
			return
		}
		value := jsonValue.GetStringBytes()
		hasValue, isInaccessible := v.definition.EnumTypeDefinitionContainsEnumValueWithDirective(fieldTypeDefinitionNode.Ref, value, federation.InaccessibleDirectiveNameBytes)
		if !hasValue || isInaccessible {
			v.renderVariableEnumValueDoesNotExistError(typeName, value)
		}
	}
}
