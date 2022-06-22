package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendObjectTypeDefinition(collectedEntities entitySet) *extendObjectTypeDefinitionVisitor {
	return &extendObjectTypeDefinitionVisitor{
		collectedEntities: collectedEntities,
	}
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	collectedEntities entitySet
}

func (e *extendObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterObjectTypeExtensionVisitor(e)
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {
	nameBytes := e.document.ObjectTypeExtensionNameBytes(ref)
	nodes, exists := e.document.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}

	var nodeToExtend *ast.Node
	isEntity := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		if nodeToExtend != nil {
			e.StopWithExternalErr(*multipleExtensionError(isEntity, nameBytes))
			return
		}
		var err *operationreport.ExternalError
		extension := e.document.ObjectTypeExtensions[ref]
		if isEntity, err = e.collectedEntities.isExtensionForEntity(nameBytes, extension.Directives.Refs, e.document); err != nil {
			e.StopWithExternalErr(*err)
			return
		}
		nodeToExtend = &nodes[i]
		if ast.IsRootType(nameBytes) {
			break
		}
	}

	if nodeToExtend == nil {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(nameBytes))
		return
	}

	e.document.ExtendObjectTypeDefinitionByObjectTypeExtension(nodeToExtend.Ref, ref)
}
