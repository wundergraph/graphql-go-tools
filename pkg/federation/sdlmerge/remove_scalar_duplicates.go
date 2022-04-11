package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateScalarTypeDefinitionVisitor struct {
	operation     *ast.Document
	scalarSet     map[string]bool
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateScalarTypeDefinitionVistior() *removeDuplicateScalarTypeDefinitionVisitor {
	return &removeDuplicateScalarTypeDefinitionVisitor{
		nil,
		make(map[string]bool),
		make([]ast.Node, 0),
		-1,
	}
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) EnterDocument(operation, definition *ast.Document) {
	r.operation = operation
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) EnterScalarTypeDefinition(ref int) {
	if ref <= r.lastRef {
		return
	}
	name := r.operation.ScalarTypeDefinitionNameString(ref)
	if ok := r.scalarSet[name]; ok {
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
	r.operation.DeleteRootNodes(r.nodesToRemove)
}

func (r *removeDuplicateScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
}
