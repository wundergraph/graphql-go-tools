package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendInterfaceTypeDefinition() *extendInterfaceTypeDefinitionVisitor {
	return &extendInterfaceTypeDefinitionVisitor{}
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendInterfaceTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(e)
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterInterfaceTypeExtension(ref int) {
	document := e.document
	nodes, exists := e.document.Index.NodesByNameBytes(document.InterfaceTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for _, node := range nodes {
		if node.Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(document.InterfaceTypeExtensionNameString(ref)))
		}
		e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}

	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(document.InterfaceTypeExtensionNameBytes(ref)))
	}
}
