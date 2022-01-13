package astnormalization

import (
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astimport"
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
	importer            astimport.Importer
	operation           *ast.Document
	definition          *ast.Document
	operationDefinition int
}

func (i *inputCoercionForListVisitor) inputCoercionForList(value ast.Value, ref, defType int) {
	l := ast.ListValue{}
	l.Refs = append(l.Refs, i.operation.ListValues[value.Ref].Refs...)
	listRef := i.operation.AddListValue(l)
	listValue := ast.Value{
		Kind: ast.ValueKindList,
		Ref:  listRef,
	}
	i.operation.AddValue(i.operation.Arguments[ref].Value)
	i.operation.Arguments[ref].Value = listValue
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

	value := i.operation.Arguments[ref].Value

	l := ast.ListValue{}
	switch value.Kind {
	case ast.ValueKindNull, ast.ValueKindVariable:
		return
	case ast.ValueKindList:
		i.inputCoercionForList(value, ref, defType)
	default:
		var latestRef = i.operation.AddValue(i.operation.Arguments[ref].Value)
		var definitionTypeRef = defType
		for {
			definitionTypeRef = i.definition.Types[definitionTypeRef].OfType
			if i.definition.Types[definitionTypeRef].TypeKind == ast.TypeKindList {
				innerList := ast.ListValue{}
				innerList.Refs = []int{latestRef}
				listRef := i.operation.AddListValue(innerList)
				listValue := ast.Value{
					Kind: ast.ValueKindList,
					Ref:  listRef,
				}
				latestRef = i.operation.AddValue(listValue)
				continue
			}
			break
		}
		l.Refs = []int{latestRef}
		listRef := i.operation.AddListValue(l)
		listValue := ast.Value{
			Kind: ast.ValueKindList,
			Ref:  listRef,
		}
		i.operation.Arguments[ref].Value = listValue
	}
}

func (i *inputCoercionForListVisitor) EnterDocument(operation, definition *ast.Document) {
	i.operation, i.definition = operation, definition
}

func (i *inputCoercionForListVisitor) EnterVariableDefinition(ref int) {
	variableNameString := i.operation.VariableDefinitionNameString(ref)
	variableDefinition, exists := i.operation.VariableDefinitionByNameAndOperation(i.operationDefinition, i.operation.VariableValueNameBytes(ref))
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
		return
	}

	var nestingDepth int
	ofType := variableTypeRef
	for {
		first := i.operation.Types[ofType]
		if first.OfType == -1 {
			break
		}
		ofType = first.OfType
		nestingDepth++
	}

	out := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(out)

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
	i.operationDefinition = ref
}
