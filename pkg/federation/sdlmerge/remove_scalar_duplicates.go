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

func newRemoveDuplicateScalarTypeDefinitionVisitor() *removeDuplicateScalarTypeDefinitionVisitor {
	return &removeDuplicateScalarTypeDefinitionVisitor{
		nil,
		make(map[string]bool),
		nil,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
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

func (r *removeDuplicateScalarTypeDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.nodesToRemove == nil {
		return
	}
	r.document.DeleteRootNodes(r.nodesToRemove)
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
}
