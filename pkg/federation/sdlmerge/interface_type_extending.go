package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
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

	var nodeToExtend *ast.Node
	isEntity := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		if nodeToExtend != nil {
			e.StopWithExternalErr(*multipleExtensionError(isEntity, nameBytes))
			return
		}
		var err *operationreport.ExternalError
		extension := e.document.InterfaceTypeExtensions[ref]
		if isEntity, err = e.collectedEntities.isExtensionForEntity(nameBytes, extension.Directives.Refs, e.document); err != nil {
			e.StopWithExternalErr(*err)
			return
		}
		nodeToExtend = &nodes[i]
	}

	if nodeToExtend == nil {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InterfaceTypeExtensionNameBytes(ref)))
		return
	}

	e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(nodeToExtend.Ref, ref)
}
