package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendUnionTypeDefinition() *extendUnionTypeDefinitionVisitor {
	return &extendUnionTypeDefinitionVisitor{}
}

type extendUnionTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendUnionTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterUnionTypeExtensionVisitor(e)
}

func (e *extendUnionTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendUnionTypeDefinitionVisitor) EnterUnionTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.UnionTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindUnionTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.operation.UnionTypeExtensionNameString(ref)))
		}
		e.operation.ExtendUnionTypeDefinitionByUnionTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}
}
