package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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
	hasExtended := false
	isEntity := false
	shouldReturn := ast.IsRootType(nameBytes)
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(*multipleExtensionError(isEntity, nameBytes))
			return
		}
		var err *operationreport.ExternalError
		extension := e.document.ObjectTypeExtensions[ref]
		if isEntity, err = e.collectedEntities.isTypeEntity(nameBytes, extension.HasDirectives, extension.Directives.Refs, e.document); err != nil {
			e.StopWithExternalErr(*err)
			return
		}
		e.document.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		if shouldReturn {
			return
		}
		hasExtended = true
	}
	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(nameBytes))
	}
}
