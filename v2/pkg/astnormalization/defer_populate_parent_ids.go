package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

// deferPopulateParentIds derives the missing parentDeferId for every
// @__defer_internal-stamped field after field merging. It must run after
// deduplicateFields, because field merging can relocate a field from one
// defer group's subtree into another selection set, so field parent may be lost
//
// If parentDeferId is already set (written by the flatten step for genuinely
// nested @defer fragments), it is left as-is: defers could be nested on the same object level
// and we need to maintain order of rendering
func deferPopulateParentIds(walker *astvisitor.Walker) {
	visitor := &deferPopulateParentIdsVisitor{Walker: walker}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
}

type deferStackEntry struct {
	id       int
	fieldRef int
}

type deferPopulateParentIdsVisitor struct {
	*astvisitor.Walker

	operation  *ast.Document
	deferStack []deferStackEntry
}

func (v *deferPopulateParentIdsVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
	v.deferStack = v.deferStack[:0]
}

func (v *deferPopulateParentIdsVisitor) EnterField(ref int) {
	id, directiveRef, exists := v.operation.FieldInternalDeferIDWithDirectiveRef(ref)
	if !exists {
		return
	}

	if len(v.deferStack) > 0 {
		if enclosing := v.deferStack[len(v.deferStack)-1].id; enclosing != id {
			// Skip if parentDeferId is already set
			if _, alreadySet := v.operation.DirectiveArgumentValueByName(directiveRef, []byte("parentDeferId")); !alreadySet {
				argRef := v.operation.AddIntArgument("parentDeferId", enclosing)
				v.operation.Directives[directiveRef].Arguments.Refs = append(v.operation.Directives[directiveRef].Arguments.Refs, argRef)
				v.operation.Directives[directiveRef].HasArguments = true
			}
		}
	}

	v.deferStack = append(v.deferStack, deferStackEntry{id: id, fieldRef: ref})
}

func (v *deferPopulateParentIdsVisitor) LeaveField(ref int) {
	if len(v.deferStack) > 0 && v.deferStack[len(v.deferStack)-1].fieldRef == ref {
		v.deferStack = v.deferStack[:len(v.deferStack)-1]
	}
}
