package astnormalization

import (
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
	walker.RegisterEnterArgumentVisitor(&visitor)
	walker.RegisterEnterVariableDefinitionVisitor(&visitor)
}

type inputCoercionForListVisitor struct {
	*astvisitor.Walker
	operation              *ast.Document
	definition             *ast.Document
	operationDefinitionRef int
}

func (i *inputCoercionForListVisitor) EnterArgument(ref int) {
	defRef, ok := i.ArgumentInputValueDefinition(ref)
	if !ok {
		return
	}

	defType := i.definition.InputValueDefinitions[defRef].Type

	// Check the definition here.
	//
	// If it is not a TypeKindList or TypeKindNonNull, stop the operation.
	// Because we only implement input coercion for lists. Please note that
	// the type kind can be a non-null list.
	definitionType := i.definition.Types[defType]
	typeKind := definitionType.TypeKind
	switch typeKind {
	case ast.TypeKindList:
	case ast.TypeKindNonNull:
		innerType := i.definition.Types[definitionType.OfType]
		// We are only looking for lists. If the definition type is
		// a non-null list, go on. Otherwise, stop the operation.
		if innerType.TypeKind != ast.TypeKindList {
			return
		}
	default:
		return
	}

	// Check the argument value kind here.
	// Only work for scalar/object types in EnterArgument.
	argumentValue := i.operation.Arguments[ref].Value
	var latestValue ast.Value
	switch argumentValue.Kind {
	case ast.ValueKindString,
		ast.ValueKindBoolean,
		ast.ValueKindInteger,
		ast.ValueKindFloat,
		ast.ValueKindObject:
		var latestRef = i.operation.AddValue(i.operation.Arguments[ref].Value)
		var definitionTypeRef = defType
		for {
			// Build a list from scalar/object type here.
			definitionType := i.definition.Types[definitionTypeRef]
			if definitionType.OfType == ast.InvalidRef {
				break
			}

			switch definitionType.TypeKind {
			case ast.TypeKindList:
				// Put the value into a list here.
				innerList := ast.ListValue{}
				innerList.Refs = []int{latestRef}
				innerListRef := i.operation.AddListValue(innerList)
				listValue := ast.Value{
					Kind: ast.ValueKindList,
					Ref:  innerListRef,
				}
				latestValue = listValue
				latestRef = i.operation.AddValue(listValue)
				definitionTypeRef = definitionType.OfType
			case ast.TypeKindNonNull:
				// Non-null type of what?
				definitionTypeRef = definitionType.OfType
			default:
			}
		}
		i.operation.Arguments[ref].Value = latestValue
	default:
		// Possible value kinds:
		// * ValueKindList
		// * ValueKindEnum
		// * ValueKindVariable
		//
		// Only works with scalar types or object types.
	}
}

func (i *inputCoercionForListVisitor) EnterDocument(operation, definition *ast.Document) {
	i.operation, i.definition = operation, definition
}

func (i *inputCoercionForListVisitor) EnterVariableDefinition(ref int) {
	variableNameString := i.operation.VariableDefinitionNameString(ref)
	variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.operationDefinitionRef, i.operation.VariableValueNameBytes(ref))
	if !exists {
		return
	}
	variableTypeRef := i.operation.VariableDefinitions[variableDefinition].Type
	variableTypeRef = i.operation.ResolveListOrNameType(variableTypeRef)

	// If the variable type is not a list, stop the operation.
	if !i.operation.TypeIsList(variableTypeRef) {
		return
	}

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
	var nestingDepth int
	ofType := variableTypeRef
	for {
		first := i.operation.Types[ofType]
		if first.OfType == ast.InvalidRef {
			break
		}

		ofType = first.OfType

		switch first.TypeKind {
		case ast.TypeKindList:
			nestingDepth++
		default:
			continue
		}
	}

	out := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(out)

	// value type is a non-array. Let's build an array from it.
	for idx := 0; idx < nestingDepth; idx++ {
		_, err := out.Write(literal.LBRACK)
		if err != nil {
			i.StopWithInternalErr(err)
			return
		}
	}

	_, err = out.Write(value)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}

	for idx := 0; idx < nestingDepth; idx++ {
		_, err = out.Write(literal.RBRACK)
		if err != nil {
			i.StopWithInternalErr(err)
			return
		}
	}

	// We built a JSON array from the given variable here.

	// Use a new slice before putting it into the variables.
	// If we use the `out` buffer here, another pool user could re-use
	// it and manipulate the variables.
	data := make([]byte, out.Len())
	copy(data, out.Bytes())
	i.operation.Input.Variables, err = sjson.SetRawBytes(i.operation.Input.Variables, variableNameString, data)
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
}

func (i *inputCoercionForListVisitor) EnterOperationDefinition(ref int) {
	i.operationDefinitionRef = ref
}
