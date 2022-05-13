package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func newExtendInterfaceTypeDefinition() *extendInterfaceTypeDefinitionVisitor {
	return &extendInterfaceTypeDefinitionVisitor{}
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendInterfaceTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(e)
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterInterfaceTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.InterfaceTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	hasExtended := false
	for _, node := range nodes {
		if node.Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.operation.InterfaceTypeExtensionNameString(ref)))
		}
		e.operation.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}
}
