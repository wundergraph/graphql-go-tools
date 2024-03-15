package astnormalization

import (
	"bytes"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
)

func mergeInlineFragmentSelections(walker *astvisitor.Walker) {
	visitor := inlineFragmentSelectionMergeVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type inlineFragmentSelectionMergeVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (f *inlineFragmentSelectionMergeVisitor) EnterDocument(operation, definition *ast.Document) {
	f.operation = operation
}

func (f *inlineFragmentSelectionMergeVisitor) fragmentsCanBeMerged(left, right int) bool {
	leftName := f.operation.InlineFragmentTypeConditionName(left)
	rightName := f.operation.InlineFragmentTypeConditionName(right)

	if !bytes.Equal(leftName, rightName) {
		return false
	}

	leftDirectives := f.operation.InlineFragmentDirectives(left)
	rightDirectives := f.operation.InlineFragmentDirectives(right)

	return f.operation.DirectiveSetsAreEqual(leftDirectives, rightDirectives)
}

func (f *inlineFragmentSelectionMergeVisitor) mergeInlineFragments(left, right int) (ok bool) {
	var leftSet, rightSet int
	leftSet, ok = f.operation.InlineFragmentSelectionSet(left)
	if !ok {
		return
	}

	rightSet, ok = f.operation.InlineFragmentSelectionSet(right)
	if !ok {
		return
	}

	f.operation.AppendSelectionSet(leftSet, rightSet)
	return true
}

func (f *inlineFragmentSelectionMergeVisitor) EnterSelectionSet(ref int) {
	if len(f.operation.SelectionSets[ref].SelectionRefs) < 2 {
		return
	}

	for _, leftSelection := range f.operation.SelectionSets[ref].SelectionRefs {
		if !f.operation.SelectionIsInlineFragmentSelection(leftSelection) {
			continue
		}
		leftInlineFragment := f.operation.Selections[leftSelection].Ref
		for i, rightSelection := range f.operation.SelectionSets[ref].SelectionRefs {
			if !f.operation.SelectionIsInlineFragmentSelection(rightSelection) {
				continue
			}
			if leftSelection == rightSelection {
				continue
			}
			rightInlineFragment := f.operation.Selections[rightSelection].Ref
			if !f.fragmentsCanBeMerged(leftInlineFragment, rightInlineFragment) {
				continue
			}
			if f.mergeInlineFragments(leftInlineFragment, rightInlineFragment) {
				f.operation.RemoveFromSelectionSet(ref, i)
				f.RevisitNode()
			}

			return
		}
	}
}
