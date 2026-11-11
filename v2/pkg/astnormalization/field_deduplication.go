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
	// merges holds the {left, right} field pairs found in one selection set, in scan order.
	merges [][2]int
}

func (d *deduplicateFieldsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
}

// EnterSelectionSet keeps the first of each group of equal leaf fields
// and merges the later ones into it.
// Merging changes only @__defer_internal, which the equality check ignores,
// so merges can run after the scan.
func (d *deduplicateFieldsVisitor) EnterSelectionSet(ref int) {
	refs := d.operation.SelectionSets[ref].SelectionRefs
	if len(refs) < 2 {
		return
	}

	d.merges = d.merges[:0]
	// For each leaf field in order, it scans the fields after it.
	// It merges equal ones into the first copy and filters them out of the list in place.
	// The next field then scans the shorter list. The scan never restarts from the top.
	for a := 0; a < len(refs); a++ {
		left, ok := d.leafField(refs[a])
		if !ok {
			continue
		}
		// Filter the tail in place; it holds only refs after a.
		kept := refs[:a+1]
		for _, j := range refs[a+1:] {
			// Directives must be fully equal, in any order.
			if right, ok := d.leafField(j); ok && d.operation.FieldsAreEqualFlat(left, right, true) {
				d.merges = append(d.merges, [2]int{left, right})
				continue
			}
			kept = append(kept, j)
		}
		refs = kept
	}

	d.operation.SelectionSets[ref].SelectionRefs = refs

	// Merge after the set is published, so Document.OnMergeFields never sees a half-filtered list.
	for _, m := range d.merges {
		d.operation.MergeFieldsDefer(m[0], m[1]) // merge right into left.
	}
}

// leafField returns the field ref of a selection that is a field without a selection set.
func (d *deduplicateFieldsVisitor) leafField(selectionRef int) (int, bool) {
	if d.operation.Selections[selectionRef].Kind != ast.SelectionKindField {
		return ast.InvalidRef, false
	}
	fieldRef := d.operation.Selections[selectionRef].Ref
	if d.operation.Fields[fieldRef].HasSelections {
		return ast.InvalidRef, false
	}
	return fieldRef, true
}
