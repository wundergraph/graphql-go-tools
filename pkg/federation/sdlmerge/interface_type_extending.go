package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func newExtendInterfaceTypeDefinition(collectedEntities entitySet) *extendInterfaceTypeDefinitionVisitor {
	return &extendInterfaceTypeDefinitionVisitor{
		collectedEntities: collectedEntities,
	}
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	collectedEntities entitySet
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
	nameBytes := e.document.InterfaceTypeExtensionNameBytes(ref)
	nodes, exists := e.document.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}
	hasExtended := false
	isEntity := false
	for _, node := range nodes {
		if node.Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(*multipleExtensionError(isEntity, nameBytes))
			return
		}
		var err *operationreport.ExternalError
		extension := e.document.InterfaceTypeExtensions[ref]
		if isEntity, err = e.collectedEntities.isTypeEntity(nameBytes, extension.HasDirectives, extension.Directives.Refs, e.document); err != nil {
			e.StopWithExternalErr(*err)
			return
		}
		e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}
	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InterfaceTypeExtensionNameBytes(ref)))
	}
}
