package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendObjectTypeDefinition(n *normalizer) *extendObjectTypeDefinitionVisitor {
	return &extendObjectTypeDefinitionVisitor{
		normalizer: n,
	}
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
}

func (e *extendObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterObjectTypeExtensionVisitor(e)
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
	e.normalizer.entityValidator.setDocument(operation)
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
			e.Walker.StopWithExternalErr(*getMultipleExtensionError(isEntity, nameBytes))
		}
		var err *operationreport.ExternalError
		extension := e.document.ObjectTypeExtensions[ref]
		if isEntity, err = e.normalizer.entityValidator.isEntity(nameBytes, extension.HasDirectives, extension.Directives.Refs, extension.FieldsDefinition.Refs); err != nil {
			e.Walker.StopWithExternalErr(*err)
		}
		e.document.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		if shouldReturn {
			return
		}
		hasExtended = true
	}
	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(nameBytes))
	}
}
