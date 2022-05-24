package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendUnionTypeDefinition() *extendUnionTypeDefinitionVisitor {
	return &extendUnionTypeDefinitionVisitor{}
}

type extendUnionTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendUnionTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterUnionTypeExtensionVisitor(e)
}

func (e *extendUnionTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendUnionTypeDefinitionVisitor) EnterUnionTypeExtension(ref int) {
	document := e.document
	nodes, exists := document.Index.NodesByNameBytes(document.UnionTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindUnionTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(document.UnionTypeExtensionNameString(ref)))
		}
		document.ExtendUnionTypeDefinitionByUnionTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}

	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(document.UnionTypeExtensionNameBytes(ref)))
	}
}
