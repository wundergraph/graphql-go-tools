package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// deduplicateFields registers a visitor to remove duplicate fields in a GraphQL operation.
func deduplicateFields(walker *astvisitor.Walker) {
	visitor := deduplicateFieldsVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type deduplicateFieldsVisitor struct {
	*astvisitor.Walker

	operation *ast.Document
}

func (d *deduplicateFieldsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
}

func (d *deduplicateFieldsVisitor) EnterSelectionSet(ref int) {
	if len(d.operation.SelectionSets[ref].SelectionRefs) < 2 {
		return
	}

	refs := d.operation.SelectionSets[ref].SelectionRefs
	for a := 0; a < len(refs); a++ {
		i := refs[a]
		if d.operation.Selections[i].Kind != ast.SelectionKindField {
			continue
		}
		left := d.operation.Selections[i].Ref
		if d.operation.Fields[left].HasSelections {
			continue
		}
		for b := a + 1; b < len(refs); b++ {
			j := refs[b]
			if d.operation.Selections[j].Kind != ast.SelectionKindField {
				continue
			}
			right := d.operation.Selections[j].Ref
			if d.operation.Fields[right].HasSelections {
				continue
			}
			// here we will check full directive equality if they are not equal, we won't deduplicate.
			// the order of directives doesn't matter if they are fully equal.
			if d.operation.FieldsAreEqualFlat(left, right, true) {
				d.operation.MergeFieldsDefer(left, right)
				d.operation.RemoveFromSelectionSet(ref, b)
				refs = d.operation.SelectionSets[ref].SelectionRefs
				b--
			}
		}
	}
}
