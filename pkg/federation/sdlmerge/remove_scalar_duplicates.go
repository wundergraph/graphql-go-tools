package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateScalarTypeDefinitionVisitor struct {
	document      *ast.Document
	scalarSet     map[string]bool
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateScalarTypeDefinitionVistior() *removeDuplicateScalarTypeDefinitionVisitor {
	return &removeDuplicateScalarTypeDefinitionVisitor{
		nil,
		make(map[string]bool),
		make([]ast.Node, 0),
		ast.InvalidRef,
	}
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) EnterScalarTypeDefinition(ref int) {
	if ref <= r.lastRef {
		return
	}
	name := r.document.ScalarTypeDefinitionNameString(ref)
	if r.scalarSet[name] {
		r.nodesToRemove = append(r.nodesToRemove, ast.Node{Kind: ast.NodeKindScalarTypeDefinition, Ref: ref})
	} else {
		r.scalarSet[name] = true
	}
	r.lastRef = ref
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) LeaveDocument(operation, definition *ast.Document) {
	if len(r.nodesToRemove) < 1 {
		return
	}
	r.document.DeleteRootNodes(r.nodesToRemove)
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
}
