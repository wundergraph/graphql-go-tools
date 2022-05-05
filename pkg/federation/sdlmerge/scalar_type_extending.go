package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func newExtendScalarTypeDefinition() *extendScalarTypeDefinitionVisitor {
	return &extendScalarTypeDefinitionVisitor{}
}

type extendScalarTypeDefinitionVisitor struct {
	operation *ast.Document
}

func (e *extendScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterScalarTypeExtensionVisitor(e)
}

func (e *extendScalarTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendScalarTypeDefinitionVisitor) EnterScalarTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.ScalarTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindScalarTypeDefinition {
			continue
		}
		e.operation.ExtendScalarTypeDefinitionByScalarTypeExtension(nodes[i].Ref, ref)
	}
}
