package astnormalization

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/tidwall/sjson"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/pkg/pool"
)

func inputCoercionForList(walker *astvisitor.Walker) {
	visitor := inputCoercionForListVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterVariableDefinitionVisitor(&visitor)
}

type inputCoercionForListVisitor struct {
	*astvisitor.Walker
	operation              *ast.Document
	definition             *ast.Document
	operationDefinitionRef int

	query []string
}

func (i *inputCoercionForListVisitor) EnterDocument(operation, definition *ast.Document) {
	i.operation, i.definition = operation, definition
}

func (i *inputCoercionForListVisitor) EnterOperationDefinition(ref int) {
	i.operationDefinitionRef = ref
}

func (i *inputCoercionForListVisitor) EnterVariableDefinition(ref int) {
	variableNameString := i.operation.VariableDefinitionNameString(ref)
	variableValueNameBytes := i.operation.VariableValueNameBytes(i.operation.VariableDefinitions[ref].VariableValue.Ref)
	variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.operationDefinitionRef, variableValueNameBytes)
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

	i.query = append(i.query, variableNameString)

	switch i.operation.Types[variableTypeRef].TypeKind {
	case ast.TypeKindList:
		i.processTypeKindList(i.operation, variableTypeRef, value, dataType)
	case ast.TypeKindNamed:
		// We build a query to insert changes to the original variable
		// Sample query: inputs.list.1.list.nested.list.1
		i.processTypeKindNamed(i.operation, i.operation.VariableDefinitions[ref].Type, value, dataType)
	}
}

func (i *inputCoercionForListVisitor) LeaveVariableDefinition(ref int) {
	i.query = i.query[:0]
}

func (i *inputCoercionForListVisitor) makeJSONArray(nestingDepth int, value []byte, dataType jsonparser.ValueType) ([]byte, error) {
	wrapValueInQuotes := dataType == jsonparser.String

	out := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(out)

	// value type is a non-array. Let's build an array from it.
	for idx := 0; idx < nestingDepth; idx++ {
		_, err := out.Write(literal.LBRACK)
		if err != nil {
			return nil, err
		}
	}

	if wrapValueInQuotes {
		_, err := out.Write(literal.QUOTE)
		if err != nil {
			return nil, err
		}
	}

	_, err := out.Write(value)
	if err != nil {
		return nil, err
	}

	if wrapValueInQuotes {
		_, err := out.Write(literal.QUOTE)
		if err != nil {
			return nil, err
		}
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

func (i *inputCoercionForListVisitor) updateQuery(path string) {
	i.query = append(i.query, path)
}

func (i *inputCoercionForListVisitor) queryPath() (path string) {
	return strings.Join(i.query, ".")
}

func (i *inputCoercionForListVisitor) popQuery() {
	if len(i.query)-1 > 0 {
		i.query = i.query[:len(i.query)-1]
	}
}

func (i *inputCoercionForListVisitor) calculateNestingDepth(document *ast.Document, typeRef int) int {
	var nestingDepth int
	for typeRef != ast.InvalidRef {
		first := document.Types[typeRef]

		typeRef = first.OfType

		switch first.TypeKind {
		case ast.TypeKindList:
			nestingDepth++
		default:
			continue
		}
	}
	return nestingDepth
}

/*
we analyzing json:

variants:

- array - find correspoding type and go to an each object
- object - find corresponding type and do deep field analysis
- plain: - do nothing

Object in depth:

Object is an InputDefinition

we iterate over objects field and trying to find corresponding field type

when we found field type:

it could be:
- NamedType

- List
when it is a list

we could have data as:
- json array - proceed recursively
- json plain - wrap into array
- json object - wrap into array and proceed recursively


*/

func (i *inputCoercionForListVisitor) walkJsonObject(inputObjDefTypeRef int, data []byte) {
	err := jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		i.updateQuery(string(key))
		defer i.popQuery()

		inputValueDefRef := i.definition.InputObjectTypeDefinitionInputValueDefinitionByName(inputObjDefTypeRef, key)
		if inputValueDefRef == ast.InvalidRef {
			return fmt.Errorf("nested field %s is not defined in any subfield of type %s", string(key), i.definition.InputObjectTypeDefinitionNameBytes(inputObjDefTypeRef))
		}

		typeRef := i.definition.ResolveListOrNameType(i.definition.InputValueDefinitionType(inputValueDefRef))

		switch i.definition.Types[typeRef].TypeKind {
		case ast.TypeKindList:
			i.processTypeKindList(i.definition, typeRef, value, dataType)
		case ast.TypeKindNamed:
			// We build a query to insert changes to the original variable
			// Sample query: inputs.list.1.list.nested.list.1
			i.processTypeKindNamed(i.definition, typeRef, value, dataType)
		}

		return nil

	})
	if err != nil {
		i.StopWithInternalErr(err)
	}
}

func (i *inputCoercionForListVisitor) walkJsonArray(document *ast.Document, listItemTypeRef int, data []byte) {
	index := -1
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, cbErr error) {
		if cbErr != nil {
			i.StopWithInternalErr(cbErr)
			return
		}
		index++

		i.updateQuery(strconv.Itoa(index))
		defer i.popQuery()

		itemTypeRef := document.ResolveListOrNameType(listItemTypeRef)

		switch document.Types[itemTypeRef].TypeKind {
		case ast.TypeKindList:
			i.processTypeKindList(document, itemTypeRef, value, dataType)
		case ast.TypeKindNamed:
			// We build a query to insert changes to the original variable
			// Sample query: inputs.list.1.list.nested.list.1
			i.processTypeKindNamed(document, itemTypeRef, value, dataType)
		}
	})

	if err != nil {
		i.StopWithInternalErr(err)
	}

}

func (i *inputCoercionForListVisitor) processTypeKindNamed(document *ast.Document, typeRef int, value []byte, dataType jsonparser.ValueType) {
	if dataType != jsonparser.Object {
		return
	}

	typeName := document.ResolveTypeNameBytes(typeRef)

	node, exist := i.definition.Index.FirstNodeByNameBytes(typeName)
	if !exist {
		return
	}

	switch node.Kind {
	case ast.NodeKindInputObjectTypeDefinition:
		i.walkJsonObject(node.Ref, value)
	case ast.NodeKindScalarTypeDefinition:
		return
	}
}

func (i *inputCoercionForListVisitor) processTypeKindList(document *ast.Document, typeRef int, value []byte, dataType jsonparser.ValueType) {
	// Build arrays from scalar/object types. If the variable type is an array or null,
	// stop the operation.
	// Take a look at that table: https://spec.graphql.org/October2021/#sec-List.Input-Coercion
	switch dataType {
	case jsonparser.Array:
		i.walkJsonArray(document, document.Types[typeRef].OfType, value)
		return
	case jsonparser.Null:
		return
	default:
	}

	// Calculate the nesting depth of variable definition
	// For example: [[Int]], nestingDepth = 2
	nestingDepth := i.calculateNestingDepth(document, typeRef)

	data, err := i.makeJSONArray(nestingDepth, value, dataType)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
	i.operation.Input.Variables, err = sjson.SetRawBytes(i.operation.Input.Variables, i.queryPath(), data)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}

	i.walkJsonArray(document, document.Types[typeRef].OfType, data)
}
