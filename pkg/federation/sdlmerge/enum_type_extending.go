package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type extendEnumTypeDefinitionVisitor struct {
	operation *ast.Document
}

func newExtendEnumTypeDefinition() *extendEnumTypeDefinitionVisitor {
	return &extendEnumTypeDefinitionVisitor{}
}

func (e *extendEnumTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterEnumTypeExtensionVisitor(e)
}

func (e *extendEnumTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendEnumTypeDefinitionVisitor) EnterEnumTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.EnumTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindEnumTypeDefinition {
			continue
		}
		e.operation.ExtendEnumTypeDefinitionByEnumTypeExtension(nodes[i].Ref, ref)
	}
}
