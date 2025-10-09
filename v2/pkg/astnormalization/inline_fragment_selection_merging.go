package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// mergeInlineFragmentSelections registers a visitor that
// merges inline fragment and field selections.
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

func (f *inlineFragmentSelectionMergeVisitor) fieldsCanMerge(left, right int) bool {
	leftName := f.operation.FieldNameBytes(left)
	rightName := f.operation.FieldNameBytes(right)
	leftAlias := f.operation.FieldAliasBytes(left)
	rightAlias := f.operation.FieldAliasBytes(right)

	if !bytes.Equal(leftName, rightName) || !bytes.Equal(leftAlias, rightAlias) {
		return false
	}

	leftDirectives := f.operation.FieldDirectives(left)
	rightDirectives := f.operation.FieldDirectives(right)

	// For fields with selections, check that all directives are equal
	// This ensures @skip, @include, @defer and @stream all match
	return f.operation.DirectiveSetsAreEqual(leftDirectives, rightDirectives)
}

func (f *inlineFragmentSelectionMergeVisitor) fieldsHaveSelections(left, right int) bool {
	return f.operation.Fields[left].HasSelections && f.operation.Fields[right].HasSelections
}

func (f *inlineFragmentSelectionMergeVisitor) mergeFields(left, right int) (ok bool) {
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

func (f *inlineFragmentSelectionMergeVisitor) EnterSelectionSet(ref int) {
	selectionRefs := f.operation.SelectionSets[ref].SelectionRefs
	if len(selectionRefs) < 2 {
		return
	}

	for i, leftSelection := range selectionRefs {
		leftKind := f.operation.SelectionKind(leftSelection)
		if leftKind != ast.SelectionKindInlineFragment && leftKind != ast.SelectionKindField {
			continue
		}
		leftRef := f.operation.Selections[leftSelection].Ref

		for j := i + 1; j < len(selectionRefs); j++ {
			rightSelection := selectionRefs[j]
			rightKind := f.operation.SelectionKind(rightSelection)
			if leftKind != rightKind {
				continue
			}

			rightRef := f.operation.Selections[rightSelection].Ref

			// Handle Inline Fragments.
			if leftKind == ast.SelectionKindInlineFragment {
				if !f.fragmentsCanBeMerged(leftRef, rightRef) {
					continue
				}
				if f.mergeInlineFragments(leftRef, rightRef) {
					f.operation.RemoveFromSelectionSet(ref, j)
					f.RevisitNode()
					return
				}
				continue
			}

			// Handle Fields.
			if !f.fieldsHaveSelections(leftRef, rightRef) {
				continue
			}
			if !f.fieldsCanMerge(leftRef, rightRef) {
				continue
			}
			if f.mergeFields(leftRef, rightRef) {
				f.operation.RemoveFromSelectionSet(ref, j)
				f.RevisitNode()
				return
			}
		}
	}
}
