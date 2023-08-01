package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func extendInterfaceTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendInterfaceTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(&visitor)
}

func extendInterfaceTypeDefinitionKeepingOrphans(walker *astvisitor.Walker) {
	visitor := extendInterfaceTypeDefinitionVisitor{
		Walker:               walker,
		keepExtensionOrphans: true,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(&visitor)
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation            *ast.Document
	keepExtensionOrphans bool
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterInterfaceTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.InterfaceTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		e.operation.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(nodes[i].Ref, ref)
		return
	}

	if e.keepExtensionOrphans {
		return
	}

	e.operation.ImportAndExtendInterfaceTypeDefinitionByInterfaceTypeExtension(ref)
}
