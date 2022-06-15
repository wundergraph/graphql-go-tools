package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendInterfaceTypeDefinition(n *normalizer) *extendInterfaceTypeDefinitionVisitor {
	return &extendInterfaceTypeDefinitionVisitor{
		normalizer: n,
	}
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
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
			e.Walker.StopWithExternalErr(*getMultipleExtensionError(isEntity, nameBytes))
		}
		var err *operationreport.ExternalError
		extension := e.document.InterfaceTypeExtensions[ref]
		if isEntity, err = e.normalizer.isTypeEntity(nameBytes, extension.HasDirectives, extension.Directives.Refs, e.document); err != nil {
			e.Walker.StopWithExternalErr(*err)
		}
		e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}
	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InterfaceTypeExtensionNameBytes(ref)))
	}
}
