package astnormalization

import (
	"bytes"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
)

var deferTypenameLiteral = []byte("__typename")

// deferAlignTypenameScope rewrites every @__defer_internal-stamped __typename
// field so its defer scope matches the object it describes (its enclosing object
// field), not the innermost @defer it was textually written in.
//
// __typename is a meta-field whose value is available exactly when its object is
// materialized, so it belongs in that object's defer scope:
//   - enclosing object is non-deferred (or there is no enclosing field): the
//     __typename must not be deferred — remove the directive so it stays in the
//     initial response and remains a literal `__typename` for type discrimination
//     and entity representation building.
//   - enclosing object is deferred under a different id: re-stamp the __typename
//     to mirror the enclosing object field's defer scope.
//   - already in the enclosing object's scope: leave as-is.
//
// Must run after deduplicateFields (so the enclosing object's final defer id is
// known) and before deferPopulateParentIds, as its own walker stage. It cannot
// share a walker with deferPopulateParentIds: that rule's EnterDocument pre-scans
// the whole tree for live defer ids, and this rule rewrites __typename ids during
// EnterField — on a shared walker the pre-scan would run before the rewrites and
// miss them. See the staging comment in astnormalization.go.
func deferAlignTypenameScope(walker *astvisitor.Walker) {
	visitor := &deferAlignTypenameScopeVisitor{Walker: walker}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
}

type deferAlignTypenameScopeVisitor struct {
	*astvisitor.Walker

	operation *ast.Document
}

func (v *deferAlignTypenameScopeVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
}

func (v *deferAlignTypenameScopeVisitor) EnterField(ref int) {
	if !bytes.Equal(v.operation.FieldNameBytes(ref), deferTypenameLiteral) {
		return
	}
	curID, directiveRef, exists := v.operation.FieldInternalDeferIDWithDirectiveRef(ref)
	if !exists {
		return
	}

	enclosingID, enclosingLabel, enclosingParent, enclosingDeferred := v.enclosingObjectFieldDefer()

	if !enclosingDeferred {
		// enclosing object is non-deferred (or there is no enclosing field):
		// __typename must stay in the initial scope.
		v.removeFieldDeferDirective(ref, directiveRef)
		return
	}

	if curID == enclosingID {
		// already in its object's defer group
		return
	}

	// align __typename to its enclosing object's defer scope
	v.removeFieldDeferDirective(ref, directiveRef)
	v.operation.AddDeferInternalDirectiveToField(ref, enclosingID, enclosingLabel, enclosingParent)
}

func (v *deferAlignTypenameScopeVisitor) LeaveField(ref int) {}

// enclosingObjectFieldDefer returns the defer info of the nearest ancestor field
// (the object the current field belongs to). deferred is false when there is no
// ancestor field or that field carries no @__defer_internal.
func (v *deferAlignTypenameScopeVisitor) enclosingObjectFieldDefer() (id int, label string, parentID int, deferred bool) {
	for _, v0 := range slices.Backward(v.Walker.Ancestors) {
		ancestor := v0
		if ancestor.Kind != ast.NodeKindField {
			continue
		}
		return v.operation.FieldDeferInfo(ancestor.Ref)
	}
	return 0, "", 0, false
}

func (v *deferAlignTypenameScopeVisitor) removeFieldDeferDirective(fieldRef, directiveRef int) {
	v.operation.Fields[fieldRef].Directives.RemoveDirectiveByRef(directiveRef)
	v.operation.Fields[fieldRef].HasDirectives = len(v.operation.Fields[fieldRef].Directives.Refs) > 0
}
