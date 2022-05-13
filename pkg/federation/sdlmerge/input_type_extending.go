package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendInputObjectTypeDefinition() *extendInputObjectTypeDefinitionVisitor {
	return &extendInputObjectTypeDefinitionVisitor{}
}

type extendInputObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendInputObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
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

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInputObjectTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.operation.InputObjectTypeExtensionNameString(ref)))
		}
		e.operation.ExtendInputObjectTypeDefinitionByInputObjectTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}
}
