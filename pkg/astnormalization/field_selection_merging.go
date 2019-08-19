package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func MergeFieldSelections(operation, definition *ast.Document) error {
	merger := FieldSelectionMerger{}
	return merger.Do(operation, definition)
}

type FieldSelectionMerger struct {
	walker  astvisitor.Walker
	visitor fieldSelectionMergeVisitor
}

func (f *FieldSelectionMerger) Do(operation, definition *ast.Document) error {

	f.visitor.operation = operation
	f.visitor.definition = definition

	err := f.walker.Visit(operation, definition, &f.visitor)
	if err != nil {
		return err
	}

	return nil
}

type fieldSelectionMergeVisitor struct {
	operation, definition *ast.Document
	depths                Depths
}

func (f *fieldSelectionMergeVisitor) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) fieldsCanMerge(left, right int) bool {
	leftName := f.operation.FieldName(left)
	rightName := f.operation.FieldName(right)
	leftAlias := f.operation.FieldAlias(left)
	rightAlias := f.operation.FieldAlias(right)
	return bytes.Equal(leftName, rightName) && bytes.Equal(leftAlias, rightAlias)
}

func (f *fieldSelectionMergeVisitor) isFieldSelection(ref int) bool {
	return f.operation.Selections[ref].Kind == ast.SelectionKindField
}

func (f *fieldSelectionMergeVisitor) fieldsHaveSelections(left, right int) bool {
	return f.operation.Fields[left].HasSelections && f.operation.Fields[right].HasSelections
}

func (f *fieldSelectionMergeVisitor) removeSelection(set, i int) {
	f.operation.SelectionSets[set].SelectionRefs = append(f.operation.SelectionSets[set].SelectionRefs[:i], f.operation.SelectionSets[set].SelectionRefs[i+1:]...)
}

func (f *fieldSelectionMergeVisitor) mergeFields(left, right int) {
	leftSet := f.operation.Fields[left].SelectionSet
	rightSet := f.operation.Fields[right].SelectionSet
	f.operation.SelectionSets[leftSet].SelectionRefs = append(f.operation.SelectionSets[leftSet].SelectionRefs, f.operation.SelectionSets[rightSet].SelectionRefs...)
	f.operation.Fields[left].Directives.Refs = append(f.operation.Fields[left].Directives.Refs, f.operation.Fields[right].Directives.Refs...)
}

func (f *fieldSelectionMergeVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {

	if len(f.operation.SelectionSets[ref].SelectionRefs) < 2 {
		return
	}

	for _, leftSelection := range f.operation.SelectionSets[ref].SelectionRefs {
		if !f.isFieldSelection(leftSelection) {
			continue
		}
		leftField := f.operation.Selections[leftSelection].Ref
		for i, rightSelection := range f.operation.SelectionSets[ref].SelectionRefs {
			if !f.isFieldSelection(rightSelection) {
				continue
			}
			if leftSelection == rightSelection {
				continue
			}
			rightField := f.operation.Selections[rightSelection].Ref
			if !f.fieldsHaveSelections(leftField, rightField) {
				continue
			}
			if !f.fieldsCanMerge(leftField, rightField) {
				continue
			}
			f.removeSelection(ref, i)
			f.mergeFields(leftField, rightField)
			return astvisitor.Instruction{
				Action: astvisitor.RevisitCurrentNode,
			}
		}
	}

	return
}

func (f *fieldSelectionMergeVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) EnterField(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveField(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) EnterInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) {

}

func (f *fieldSelectionMergeVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) {

}
