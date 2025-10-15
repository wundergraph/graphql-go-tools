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

// UploadFinder is a helper to find upload structs in the arguments of a query
// it can be used only in context of arguments extraction normalization rule
// because it iterates one argument at time
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
	VariableName       string // is a variable name holding the direct or nested value of type Upload, example "f"
	OriginalUploadPath string // is a path relative to variables which have an Upload type, example "variables.f"
	NewUploadPath      string // if variable was used in the inline object like this `arg: {f: $f}` this field will hold the new extracted path, example "variables.a.f", if it is an empty, there was no change in the path
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

	node, ok := v.definition.NodeByName(uploadScalarName)
	if !ok {
		// there is no Upload type found in the schema
		return
	}

	if node.Kind != ast.NodeKindScalarTypeDefinition {
		// Upload type is not a scalar type
		return
	}

	if variables == nil || bytes.Equal(variables, []byte("null")) || bytes.Equal(variables, []byte("")) {
		variables = []byte("{}")
	}

	v.variables, err = astjson.ParseBytes(variables)
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

// traverseValue is a recursive function that traverses ast.Value aka input object representation passed to a query field argument
func (v *UploadFinder) traverseValue(value ast.Value, valueTypeRef int) {
	switch value.Kind {
	case ast.ValueKindVariable: // when direct value of an argument is a variable
		v.variableUsedDirectlyOnArg = !v.currentArgPath.hasPath()
		v.traverseVariable(value.Ref, valueTypeRef)
	case ast.ValueKindList: // when value is a list we will be traversing each list item
		listItemTypeRef := valueTypeRef
		for i, ref := range v.operation.ListValues[value.Ref].Refs {
			// during traversion a list we track list index in the path
			v.currentArgPath.pushArrayPath(i)
			v.traverseValue(v.operation.Value(ref), v.definition.Types[listItemTypeRef].OfType)
			v.currentArgPath.popPath()
		}
	case ast.ValueKindObject: // when value is an object, we need to traverse it's fields
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

			// during traversion of an object we track object field name in the path
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
	// we need to get a variable typename from operation to be able to find corresponding type in the definition
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
	// once we have found a variable we need to traverse its value
	// but this time we will be walking on the definition types and look into json to see which fields are present
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
