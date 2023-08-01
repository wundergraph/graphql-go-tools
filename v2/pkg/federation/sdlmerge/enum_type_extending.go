package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type extendEnumTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func newExtendEnumTypeDefinition() *extendEnumTypeDefinitionVisitor {
	return &extendEnumTypeDefinitionVisitor{}
}

func (e *extendEnumTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterEnumTypeExtensionVisitor(e)
}

func (e *extendEnumTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendEnumTypeDefinitionVisitor) EnterEnumTypeExtension(ref int) {
	nodes, exists := e.document.Index.NodesByNameBytes(e.document.EnumTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindEnumTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.EnumTypeExtensionNameString(ref)))
			return
		}
		e.document.ExtendEnumTypeDefinitionByEnumTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}

	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.EnumTypeExtensionNameBytes(ref)))
	}
}
