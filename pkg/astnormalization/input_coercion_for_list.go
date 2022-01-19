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
	typeKind := i.definition.Types[defType].TypeKind
	if typeKind != ast.TypeKindList {
		return
	}

	argumentValue := i.operation.Arguments[ref].Value

	argumentListValue := ast.ListValue{}
	switch argumentValue.Kind {
	case ast.ValueKindString,
		ast.ValueKindBoolean,
		ast.ValueKindInteger,
		ast.ValueKindFloat,
		ast.ValueKindObject:
		var latestRef = i.operation.AddValue(i.operation.Arguments[ref].Value)
		var definitionTypeRef = defType
		for {
			definitionTypeRef = i.definition.Types[definitionTypeRef].OfType
			if i.definition.Types[definitionTypeRef].TypeKind != ast.TypeKindList {
				break
			}

			// Build a nested list
			innerList := ast.ListValue{}
			innerList.Refs = []int{latestRef}
			innerListRef := i.operation.AddListValue(innerList)
			listValue := ast.Value{
				Kind: ast.ValueKindList,
				Ref:  innerListRef,
			}
			latestRef = i.operation.AddValue(listValue)
		}
		argumentListValue.Refs = []int{latestRef}
		listRef := i.operation.AddListValue(argumentListValue)
		listValue := ast.Value{
			Kind: ast.ValueKindList,
			Ref:  listRef,
		}
		i.operation.Arguments[ref].Value = listValue
	default:
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

	if !i.operation.TypeIsList(variableTypeRef) {
		return
	}

	value, dataType, _, err := jsonparser.Get(i.operation.Input.Variables, variableNameString)
	if err == jsonparser.KeyPathNotFoundError {
		return
	}
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
	if dataType == jsonparser.Array {
		// We don't want to build nested lists using lists.
		// It's an invalid input.
		return
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
		nestingDepth++
	}

	out := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(out)

	// value type is a non-array. Let's build an array from it.
	for idx := 0; idx < (nestingDepth*2)+1; idx++ {
		switch {
		case idx < nestingDepth:
			out.Write(literal.LBRACK)
		case idx == nestingDepth:
			out.Write(value)
		default:
			out.Write(literal.RBRACK)
		}
	}

	i.operation.Input.Variables, err = sjson.SetRawBytes(i.operation.Input.Variables, variableNameString, out.Bytes())
	if err != nil {
		i.StopWithInternalErr(err)
		return
	}
}

func (i *inputCoercionForListVisitor) EnterOperationDefinition(ref int) {
	i.operationDefinitionRef = ref
}
