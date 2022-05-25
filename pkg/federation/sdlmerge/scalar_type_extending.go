package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendScalarTypeDefinition() *extendScalarTypeDefinitionVisitor {
	return &extendScalarTypeDefinitionVisitor{}
}

type extendScalarTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterScalarTypeExtensionVisitor(e)
}

func (e *extendScalarTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendScalarTypeDefinitionVisitor) EnterScalarTypeExtension(ref int) {
	nodes, exists := e.document.Index.NodesByNameBytes(e.document.ScalarTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindScalarTypeDefinition {
			continue
		}
		if hasExtended {
			e.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.ScalarTypeExtensionNameString(ref)))
			return
		}
		e.document.ExtendScalarTypeDefinitionByScalarTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}
	if !hasExtended {
		e.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.ScalarTypeExtensionNameBytes(ref)))
	}
}
