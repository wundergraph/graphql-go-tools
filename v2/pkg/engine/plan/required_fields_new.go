package plan

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func AddRequiredFields(key, operation, definition *ast.Document, operationSelectionSet int, report *operationreport.Report) {
	walker := astvisitor.NewWalker(48)

	visitor := &requiredFieldsVisitorNew{
		Walker:                &walker,
		operation:             operation,
		operationSelectionSet: operationSelectionSet,
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)
	walker.RegisterEnterSelectionSetVisitor(visitor)

	walker.Walk(key, definition, report)
	if report.HasErrors() {
		panic(report.Error())
	}
}

type requiredFieldsVisitorNew struct {
	*astvisitor.Walker
	key                   *ast.Document
	operation             *ast.Document
	operationSelectionSet int
	OperationNodes        []ast.Node
}

func (v *requiredFieldsVisitorNew) EnterDocument(key, _ *ast.Document) {
	v.key = key
	v.OperationNodes = make([]ast.Node, 0, 3)
	v.OperationNodes = append(v.OperationNodes,
		ast.Node{Kind: ast.NodeKindSelectionSet, Ref: v.operationSelectionSet})
}

func (v *requiredFieldsVisitorNew) EnterSelectionSet(ref int) {
	if v.Walker.Depth == 2 {
		return
	}

	selectionSetNode := v.operation.AddSelectionSet()

	fieldNode := v.OperationNodes[len(v.OperationNodes)-1]
	v.operation.Fields[fieldNode.Ref].HasSelections = true
	v.operation.Fields[fieldNode.Ref].SelectionSet = selectionSetNode.Ref

	v.OperationNodes = append(v.OperationNodes, selectionSetNode)
}

func (v *requiredFieldsVisitorNew) LeaveSelectionSet(ref int) {
	if v.Walker.Depth == 0 {
		return
	}

	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitorNew) EnterField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)

	selectionSetRef := v.OperationNodes[len(v.OperationNodes)-1].Ref

	if v.operation.SelectionSetHasFieldSelectionWithNameOrAliasBytes(selectionSetRef, fieldName) {
		return
	}

	fieldNode := v.addRequiredField(fieldName, selectionSetRef)
	if v.key.FieldHasSelections(ref) {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

func (v *requiredFieldsVisitorNew) LeaveField(ref int) {
	if v.key.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}
}

func (v *requiredFieldsVisitorNew) addRequiredField(fieldName ast.ByteSlice, selectionSet int) ast.Node {
	field := ast.Field{
		Name:         v.operation.Input.AppendInputBytes(fieldName),
		SelectionSet: ast.InvalidRef,
	}
	addedField := v.operation.AddField(field)

	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedField.Ref,
	}
	v.operation.AddSelection(selectionSet, selection)

	directiveRef := v.operation.AddDirective(ast.Directive{
		Name: v.operation.Input.AppendInputBytes(literal.INTERNAL_SKIP),
	})
	v.operation.AddDirectiveToNode(directiveRef, ast.Node{Kind: ast.NodeKindField, Ref: addedField.Ref})

	return addedField
}
