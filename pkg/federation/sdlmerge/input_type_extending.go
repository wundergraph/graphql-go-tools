package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func newExtendInputObjectTypeDefinition() *extendInputObjectTypeDefinitionVisitor {
	return &extendInputObjectTypeDefinitionVisitor{}
}

type extendInputObjectTypeDefinitionVisitor struct {
	operation *ast.Document
}

func (e *extendInputObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterInputObjectTypeExtensionVisitor(e)
}

func (e *extendInputObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendInputObjectTypeDefinitionVisitor) EnterInputObjectTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.InputObjectTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInputObjectTypeDefinition {
			continue
		}
		e.operation.ExtendInputObjectTypeDefinitionByInputObjectTypeExtension(nodes[i].Ref, ref)
	}
}
