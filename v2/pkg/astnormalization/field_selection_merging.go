package astnormalization

import (
	"bytes"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
)

func mergeFieldSelections(walker *astvisitor.Walker) {
	visitor := fieldSelectionMergeVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type fieldSelectionMergeVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (f *fieldSelectionMergeVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
}

func (f *fieldSelectionMergeVisitor) fieldsCanMerge(left, right int) bool {
	leftName := f.operation.FieldNameBytes(left)
	rightName := f.operation.FieldNameBytes(right)
	leftAlias := f.operation.FieldAliasBytes(left)
	rightAlias := f.operation.FieldAliasBytes(right)

	if !bytes.Equal(leftName, rightName) || !bytes.Equal(leftAlias, rightAlias) {
		return false
	}

	leftDirectives := f.operation.FieldDirectives(left)
	rightDirectives := f.operation.FieldDirectives(right)

	return f.operation.DirectiveSetsAreEqual(leftDirectives, rightDirectives)
}

func (f *fieldSelectionMergeVisitor) fieldsHaveSelections(left, right int) bool {
	return f.operation.Fields[left].HasSelections && f.operation.Fields[right].HasSelections
}

func (f *fieldSelectionMergeVisitor) mergeFields(left, right int) (ok bool) {
	var leftSet, rightSet int
	leftSet, ok = f.operation.FieldSelectionSet(left)
	if !ok {
		return false
	}
	rightSet, ok = f.operation.FieldSelectionSet(right)
	if !ok {
		return false
	}

	f.operation.AppendSelectionSet(leftSet, rightSet)
	return true
}

func (f *fieldSelectionMergeVisitor) EnterSelectionSet(ref int) {

	if len(f.operation.SelectionSets[ref].SelectionRefs) < 2 {
		return
	}

	for _, leftSelection := range f.operation.SelectionSets[ref].SelectionRefs {
		if !f.operation.SelectionIsFieldSelection(leftSelection) {
			continue
		}
		leftField := f.operation.Selections[leftSelection].Ref
		for i, rightSelection := range f.operation.SelectionSets[ref].SelectionRefs {
			if !f.operation.SelectionIsFieldSelection(rightSelection) {
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

			if f.mergeFields(leftField, rightField) {
				f.operation.RemoveFromSelectionSet(ref, i)
				f.RevisitNode()
			}
			return
		}
	}
}
