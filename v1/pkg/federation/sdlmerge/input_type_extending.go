package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendInputObjectTypeDefinition() *extendInputObjectTypeDefinitionVisitor {
	return &extendInputObjectTypeDefinitionVisitor{}
}

type extendInputObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendInputObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterInputObjectTypeExtensionVisitor(e)
}

func (e *extendInputObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendInputObjectTypeDefinitionVisitor) EnterInputObjectTypeExtension(ref int) {
	nodes, exists := e.document.Index.NodesByNameBytes(e.document.InputObjectTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInputObjectTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.InputObjectTypeExtensionNameString(ref)))
			return
		}
		e.document.ExtendInputObjectTypeDefinitionByInputObjectTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}

	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InputObjectTypeExtensionNameBytes(ref)))
	}
}
