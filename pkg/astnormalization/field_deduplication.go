package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func DeDuplicateFields(operation, definition *ast.Document) error {
	fieldDeDuplicate := FieldDeduplicate{}
	return fieldDeDuplicate.Do(operation, definition)
}

type FieldDeduplicate struct {
	walker  astvisitor.Walker
	visitor fieldDeDuplicateVisitor
}

func (f *FieldDeduplicate) Do(operation, definition *ast.Document) error {

	f.visitor.err = nil
	f.visitor.operation = operation
	f.visitor.definition = definition

	err := f.walker.Visit(operation, definition, &f.visitor)
	if err == nil {
		err = f.visitor.err
	}

	return err
}

type fieldDeDuplicateVisitor struct {
	operation, definition *ast.Document
	err                   error
}

func (f *fieldDeDuplicateVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	if len(f.operation.SelectionSets[ref].SelectionRefs) < 2 {
		return
	}
	for a, i := range f.operation.SelectionSets[ref].SelectionRefs {
		if f.operation.Selections[i].Kind != ast.SelectionKindField {
			continue
		}
		left := f.operation.Selections[i].Ref
		if f.operation.Fields[left].HasSelections {
			continue
		}
		for b, j := range f.operation.SelectionSets[ref].SelectionRefs {
			if a == b {
				continue
			}
			if a > b {
				continue
			}
			if f.operation.Selections[j].Kind != ast.SelectionKindField {
				continue
			}
			right := f.operation.Selections[j].Ref
			if f.operation.Fields[right].HasSelections {
				continue
			}
			if f.operation.FieldsAreEqualFlat(left, right) {
				f.operation.RemoveFromSelectionSet(ref, b)
				return astvisitor.Instruction{
					Action: astvisitor.RevisitCurrentNode,
				}
			}
		}
	}

	return
}

func (f *fieldDeDuplicateVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterField(ref int, info astvisitor.Info) {
}

func (f *fieldDeDuplicateVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldDeDuplicateVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
