package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendScalarTypeDefinition() *extendScalarTypeDefinitionVisitor {
	return &extendScalarTypeDefinitionVisitor{}
}

type extendScalarTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendScalarTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterScalarTypeExtensionVisitor(e)
}

func (e *extendScalarTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendScalarTypeDefinitionVisitor) EnterScalarTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.ScalarTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindScalarTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.operation.ScalarTypeExtensionNameString(ref)))
		}
		e.operation.ExtendScalarTypeDefinitionByScalarTypeExtension(nodes[i].Ref, ref)
		hasExtended = true
	}
}
