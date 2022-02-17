package astnormalization

import (
	"fmt"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/pool"
	"github.com/tidwall/sjson"
)

func inputCoercionForList(walker *astvisitor.Walker) {
	visitor := inputCoercionForListVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterVariableDefinitionVisitor(&visitor)
}

type inputCoercionForListVisitor struct {
	*astvisitor.Walker
	operation              *ast.Document
	definition             *ast.Document
	operationDefinitionRef int
}

func (i *inputCoercionForListVisitor) EnterDocument(operation, definition *ast.Document) {
	i.operation, i.definition = operation, definition
}

func (i *inputCoercionForListVisitor) inspectInputFieldType(ref int, name string) (ast.TypeKind, int) {
	typeName := i.operation.ResolveTypeNameBytes(i.operation.VariableDefinitions[ref].Type)
	node, exist := i.definition.Index.FirstNodeByNameBytes(typeName)
	if !exist {
		return ast.TypeKindUnknown, ast.InvalidRef
	}

	for _, inputDefRef := range i.definition.InputObjectTypeDefinitions[node.Ref].InputFieldsDefinition.Refs {
		typeRef := i.definition.InputValueDefinitions[inputDefRef].Type
		typeName := i.definition.ResolveTypeNameString(typeRef)
		_, ok := i.definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			break
		}
		typeDef := i.definition.Types[typeRef]
		if typeDef.TypeKind == ast.TypeKindNonNull {
			typeDef = i.definition.Types[typeDef.OfType]
		}
		if i.definition.InputValueDefinitionNameString(inputDefRef) == name {
			// We need type reference in the caller to calculate nesting depth of the list.
			return typeDef.TypeKind, typeRef
		}
	}

	return ast.TypeKindUnknown, ast.InvalidRef
}

func (i *inputCoercionForListVisitor) makeJSONArray(nestingDepth int, value []byte) ([]byte, error) {
	out := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(out)

	// value type is a non-array. Let's build an array from it.
	for idx := 0; idx < nestingDepth; idx++ {
		_, err := out.Write(literal.LBRACK)
		if err != nil {
			return nil, err
		}
	}

	_, err := out.Write(value)
	if err != nil {
		return nil, err
	}

	for idx := 0; idx < nestingDepth; idx++ {
		_, err = out.Write(literal.RBRACK)
		if err != nil {
			return nil, err
		}
	}

	// We built a JSON array from the given variable here.

	// Use a new slice before putting it into the variables.
	// If we use the `out` buffer here, another pool user could re-use
	// it and manipulate the variables.
	data := make([]byte, out.Len())
	copy(data, out.Bytes())
	return data, nil
}

func (i *inputCoercionForListVisitor) updateQuery(dataType jsonparser.ValueType, query, path string) string {
	if dataType == jsonparser.Array || dataType == jsonparser.Object {
		query = fmt.Sprintf("%s.%s", query, path)
	}
	return query
}

func (i *inputCoercionForListVisitor) calculateNestingDepth(ref int) int {
	var nestingDepth int
	for ref != ast.InvalidRef {
		first := i.definition.Types[ref]

		ref = first.OfType

		switch first.TypeKind {
		case ast.TypeKindList:
			nestingDepth++
		default:
			continue
		}
	}
	return nestingDepth
}

func (i *inputCoercionForListVisitor) processVariableTypeKindNamed(query string, ref int, data []byte, dataType jsonparser.ValueType) {
	var err error
	switch dataType {
	case jsonparser.Object:
		err = jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			query = i.updateQuery(dataType, query, string(key))

			typeKind, typeRef := i.inspectInputFieldType(ref, string(key))

			// The errors returned by the callback function are handled by jsonparser.ObjectEach
			if typeKind == ast.TypeKindList && dataType != jsonparser.Array {
				nestingDepth := i.calculateNestingDepth(typeRef)
				value, err := i.makeJSONArray(nestingDepth, value)
				if err != nil {
					return err
				}
				i.operation.Input.Variables, err = sjson.SetRawBytes(i.operation.Input.Variables, query, value)
				if err != nil {
					return err
				}

				i.processVariableTypeKindNamed(query, ref, value, jsonparser.Array)
			} else {
				i.processVariableTypeKindNamed(query, ref, value, dataType)
			}
			return nil
		})
	case jsonparser.Array:
		_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, cbErr error) {
			if cbErr != nil {
				i.StopWithInternalErr(cbErr)
				return
			}

			query = i.updateQuery(dataType, query, strconv.Itoa(offset-1))
			i.processVariableTypeKindNamed(query, ref, value, dataType)
		})
	}
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
}

func (i *inputCoercionForListVisitor) processVariableTypeKindList(variableTypeRef int, value []byte, variableNameString string, dataType jsonparser.ValueType) {
	// Build arrays from scalar/object types. If the variable type is an array or null,
	// stop the operation.
	// Take a look at that table: https://spec.graphql.org/October2021/#sec-List.Input-Coercion
	switch dataType {
	case jsonparser.Array,
		jsonparser.Null:
		return
	default:
	}

	// Calculate the nesting depth of variable definition
	// For example: [[Int]], nestingDepth = 2
	ofType := variableTypeRef
	var nestingDepth int
	for ofType != ast.InvalidRef {
		first := i.operation.Types[ofType]
		ofType = first.OfType
		switch first.TypeKind {
		case ast.TypeKindList:
			nestingDepth++
		default:
			continue
		}
	}

	data, err := i.makeJSONArray(nestingDepth, value)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
	i.operation.Input.Variables, err = sjson.SetRawBytes(i.operation.Input.Variables, variableNameString, data)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
}

func (i *inputCoercionForListVisitor) EnterVariableDefinition(ref int) {
	variableNameString := i.operation.VariableDefinitionNameString(ref)
	variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.operationDefinitionRef, i.operation.VariableValueNameBytes(ref))
	if !exists {
		return
	}
	variableTypeRef := i.operation.VariableDefinitions[variableDefinition].Type
	variableTypeRef = i.operation.ResolveListOrNameType(variableTypeRef)

	value, dataType, _, err := jsonparser.Get(i.operation.Input.Variables, variableNameString)
	if err == jsonparser.KeyPathNotFoundError {
		// If the user doesn't provide any variable with that name,
		// there is no need for coercion. Stop the operation
		return
	}
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}

	switch i.operation.Types[variableTypeRef].TypeKind {
	case ast.TypeKindList:
		i.processVariableTypeKindList(variableTypeRef, value, variableNameString, dataType)
	case ast.TypeKindNamed:
		// We build a query to insert changes to the original variable
		// Sample query: inputs.list.1.list.nested.list.1
		query := variableNameString
		i.processVariableTypeKindNamed(query, ref, value, dataType)
	}
}

func (i *inputCoercionForListVisitor) EnterOperationDefinition(ref int) {
	i.operationDefinitionRef = ref
}
