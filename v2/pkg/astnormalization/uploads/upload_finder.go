package uploads

import (
	"bytes"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

var (
	uploadScalarName = []byte("Upload")
	variablesLiteral = []byte("variables")
)

type UploadFinder struct {
	operation                 *ast.Document
	definition                *ast.Document
	variables                 *astjson.Value
	currentVariableName       []byte
	currentVariableValue      *astjson.Value
	currentArgPath            *Path
	currentVariablePath       *Path
	variableUsedDirectlyOnArg bool

	uploadPathMapping []UploadPathMapping
}

type UploadPathMapping struct {
	VariableName       string
	OriginalUploadPath string
	NewUploadPath      string
}

func NewUploadFinder() *UploadFinder {
	return &UploadFinder{
		currentArgPath:      &Path{},
		currentVariablePath: &Path{},
	}
}

func (v *UploadFinder) Reset() {
	v.operation = nil
	v.definition = nil
	v.variables = nil
	v.currentVariableName = nil
	v.currentVariableValue = nil
	v.currentArgPath.reset()
	v.currentVariablePath.reset()
	v.variableUsedDirectlyOnArg = false
	v.uploadPathMapping = v.uploadPathMapping[:0]
}

func (v *UploadFinder) FindUploads(operation, definition *ast.Document, variables []byte, argRef int, argInputValueDefinitionRef int) (uploadPathMapping []UploadPathMapping, err error) {
	v.definition = definition
	v.operation = operation

	if variables == nil || bytes.Equal(variables, []byte("null")) || bytes.Equal(variables, []byte("")) {
		variables = []byte("{}")
	}

	v.variables, err = astjson.ParseBytesWithoutCache(variables)
	if err != nil {
		return nil, err
	}

	v.currentArgPath.reset()
	v.currentVariablePath.reset()

	argumentValue := v.operation.Arguments[argRef].Value
	argumentTypeRef := v.definition.InputValueDefinitionType(argInputValueDefinitionRef)

	v.traverseValue(argumentValue, argumentTypeRef)

	if len(v.uploadPathMapping) > 0 {
		return v.uploadPathMapping, nil
	}

	return nil, nil
}

func (v *UploadFinder) traverseValue(value ast.Value, valueTypeRef int) {
	switch value.Kind {
	case ast.ValueKindVariable:
		v.variableUsedDirectlyOnArg = !v.currentArgPath.hasPath()
		v.traverseVariable(value.Ref, valueTypeRef)
	case ast.ValueKindList:
		listItemTypeRef := valueTypeRef
		for i, ref := range v.operation.ListValues[value.Ref].Refs {
			v.currentArgPath.pushArrayPath(i)
			v.traverseValue(v.operation.Value(ref), v.definition.Types[listItemTypeRef].OfType)
			v.currentArgPath.popPath()
		}
	case ast.ValueKindObject:
		typeNameBytes := v.definition.ResolveTypeNameBytes(valueTypeRef)
		objectTypeNode, ok := v.definition.NodeByName(typeNameBytes)
		if !ok {
			return
		}
		if objectTypeNode.Kind != ast.NodeKindInputObjectTypeDefinition {
			return
		}

		for _, ref := range v.operation.ObjectValues[value.Ref].Refs {
			objectFieldName := v.operation.ObjectFieldNameBytes(ref)

			fieldInputValueDefinitionRef, ok := v.definition.InputValueDefinitionRefByInputObjectDefinitionRefAndFieldNameBytes(objectTypeNode.Ref, objectFieldName)
			if !ok {
				continue
			}

			v.currentArgPath.pushObjectPath(objectFieldName)
			v.traverseValue(v.operation.ObjectField(ref).Value, v.definition.InputValueDefinitionType(fieldInputValueDefinitionRef))
			v.currentArgPath.popPath()
		}
	case ast.ValueKindNull:
		// we care only about null - because it could be an upload
		if v.definition.ResolveTypeNameString(valueTypeRef) == "Upload" {
			v.AddUploadPath()
		}
	}
}

func (v *UploadFinder) traverseVariable(variableValueRef int, definitionValueTypeRef int) {
	operationVariableNameBytes := v.operation.VariableValueNameBytes(variableValueRef)
	operationVariableName := unsafebytes.BytesToString(operationVariableNameBytes)

	v.currentVariableName = operationVariableNameBytes
	v.currentVariableValue = v.variables.Get(operationVariableName)

	// we rely on the fact that there is only one operation in the document
	const operationDefinitionRef = 0
	variableDefinitionRef, ok := v.operation.VariableDefinitionByNameAndOperation(operationDefinitionRef, operationVariableNameBytes)
	if !ok {
		return
	}

	variableTypeRef := v.operation.VariableDefinitionType(variableDefinitionRef)

	variableTypeName := v.operation.ResolveTypeNameBytes(variableTypeRef)
	definitionTypeName := v.definition.ResolveTypeNameBytes(definitionValueTypeRef)
	if !bytes.Equal(variableTypeName, definitionTypeName) {
		return
	}

	v.currentVariablePath.pushObjectPath(variablesLiteral)
	v.currentVariablePath.pushObjectPath(operationVariableNameBytes)
	v.traverseOperationType(v.currentVariableValue, variableTypeRef)
	v.currentVariablePath.popPath()
	v.currentVariablePath.popPath()
}

func (v *UploadFinder) traverseOperationType(jsonValue *astjson.Value, operationTypeRef int) {
	if jsonValue == nil {
		return
	}

	if v.operation.TypeIsNonNull(operationTypeRef) {
		v.traverseOperationType(jsonValue, v.operation.Types[operationTypeRef].OfType)
		return
	}

	if v.operation.TypeIsList(operationTypeRef) {
		// TODO: current implementation is not aware of the list input coercion
		if jsonValue.Type() != astjson.TypeArray {
			return
		}
		values := jsonValue.GetArray()
		for i, arrayValue := range values {
			v.currentArgPath.pushArrayPath(i)
			v.currentVariablePath.pushArrayPath(i)
			v.traverseOperationType(arrayValue, v.operation.Types[operationTypeRef].OfType)
			v.currentArgPath.popPath()
			v.currentVariablePath.popPath()
			continue
		}
		return
	}

	varTypeName := v.operation.ResolveTypeNameBytes(operationTypeRef)
	if jsonValue.Type() == astjson.TypeNull && varTypeName.String() == "Upload" {
		v.AddUploadPath()
		return
	}

	v.traverseNamedTypeNode(jsonValue, varTypeName)
}

func (v *UploadFinder) traverseFieldDefinitionType(fieldTypeDefinitionNodeKind ast.NodeKind, fieldName ast.ByteSlice, jsonValue *astjson.Value, typeRef, inputFieldRef int) {
	if v.definition.TypeIsNonNull(typeRef) {
		v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, jsonValue, v.definition.Types[typeRef].OfType, inputFieldRef)
		return
	}

	if v.definition.TypeIsList(typeRef) {
		// TODO: current implementation is not aware of the list input coercion
		// so basically it could be the case that json value here will be an object or a single scalar
		if jsonValue.Type() != astjson.TypeArray {
			return
		}
		if len(jsonValue.GetArray()) == 0 {
			return
		}

		for i, arrayValue := range jsonValue.GetArray() {
			v.currentArgPath.pushArrayPath(i)
			v.currentVariablePath.pushArrayPath(i)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNodeKind, fieldName, arrayValue, v.definition.Types[typeRef].OfType, inputFieldRef)
			v.currentArgPath.popPath()
			v.currentVariablePath.popPath()
			continue
		}
		return
	}

	v.traverseNamedTypeNode(jsonValue, v.definition.ResolveTypeNameBytes(typeRef))
}

func (v *UploadFinder) traverseNamedTypeNode(jsonValue *astjson.Value, typeName []byte) {
	fieldTypeDefinitionNode, ok := v.definition.NodeByName(typeName)
	if !ok {
		return
	}
	switch fieldTypeDefinitionNode.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		if jsonValue.Type() != astjson.TypeObject {
			return
		}
		inputFieldRefs := v.definition.NodeInputFieldDefinitions(fieldTypeDefinitionNode)
		for _, inputFieldRef := range inputFieldRefs {
			fieldName := v.definition.InputValueDefinitionNameBytes(inputFieldRef)
			fieldTypeRef := v.definition.InputValueDefinitionType(inputFieldRef)
			objectFieldValue := jsonValue.Get(unsafebytes.BytesToString(fieldName))

			if objectFieldValue == nil {
				// we don't have such field in the json object
				continue
			}

			v.currentArgPath.pushObjectPath(fieldName)
			v.currentVariablePath.pushObjectPath(fieldName)
			v.traverseFieldDefinitionType(fieldTypeDefinitionNode.Kind, fieldName, objectFieldValue, fieldTypeRef, inputFieldRef)
			v.currentArgPath.popPath()
			v.currentVariablePath.popPath()
		}
	case ast.NodeKindScalarTypeDefinition:
		if bytes.Equal(typeName, uploadScalarName) && jsonValue.Type() == astjson.TypeNull {
			v.AddUploadPath()
		}
	default:
	}
}

func (v *UploadFinder) AddUploadPath() {
	if v.variableUsedDirectlyOnArg {
		v.uploadPathMapping = append(v.uploadPathMapping, UploadPathMapping{
			OriginalUploadPath: v.currentVariablePath.render(),
			NewUploadPath:      "",
			VariableName:       string(v.currentVariableName),
		})
		return
	}

	v.uploadPathMapping = append(v.uploadPathMapping, UploadPathMapping{
		OriginalUploadPath: v.currentVariablePath.render(),
		NewUploadPath:      v.currentArgPath.render(),
		VariableName:       string(v.currentVariableName),
	})
}
