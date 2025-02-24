package astnormalization

import (
	"bytes"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// selectionSetFieldsCompare should return a negative number when doc.Selections[sRefA] < doc.Selections[sRefB],
// a positive number when doc.Selections[sRefA] > doc.Selections[sRefB]
// and zero when doc.Selections[sRefA] == doc.Selections[sRefB]
type selectionSetFieldsCompare func(doc *ast.Document, sRefA, sRefB int) int

func sortSelectionSetFields(walker *astvisitor.Walker, compareFn selectionSetFieldsCompare) {
	visitor := sortSelectionSetFieldsVisitor{
		Walker:    walker,
		compareFn: compareFn,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type sortSelectionSetFieldsVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
	compareFn selectionSetFieldsCompare
}

func (s *sortSelectionSetFieldsVisitor) EnterDocument(operation, _ *ast.Document) {
	s.operation = operation
}

func CompareSelectionSetFieldsLexicographically(doc *ast.Document, sRefA, sRefB int) int {
	getLexiValue := func(sRef int) []byte {
		if doc.SelectionIsFieldSelection(sRef) {
			return doc.FieldAliasOrNameBytes(doc.Selections[sRef].Ref)
		} else if doc.SelectionIsInlineFragmentSelection(sRef) {
			return doc.TypeNameBytes(doc.InlineFragments[doc.Selections[sRef].Ref].TypeCondition.Type)
		} else if doc.SelectionIsFragmentSpreadSelection(sRef) {
			return doc.FragmentSpreadNameBytes(doc.Selections[sRef].Ref)
		} else {
			return nil
		}
	}

	return bytes.Compare(getLexiValue(sRefA), getLexiValue(sRefB))
}

func (s *sortSelectionSetFieldsVisitor) EnterSelectionSet(ssRef int) {
	if len(s.operation.SelectionSets[ssRef].SelectionRefs) < 2 {
		// nothing to sort
		return
	}

	slices.SortStableFunc(s.operation.SelectionSets[ssRef].SelectionRefs, func(sRefA, sRefB int) int {
		return s.compareFn(s.operation, sRefA, sRefB)
	})
}
