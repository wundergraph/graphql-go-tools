package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func extendObjectTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendObjectTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterObjectTypeExtensionVisitor(&visitor)
}

func extendObjectTypeDefinitionKeepingOrphans(walker *astvisitor.Walker) {
	visitor := extendObjectTypeDefinitionVisitor{
		Walker:               walker,
		keepExtensionOrphans: true,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterObjectTypeExtensionVisitor(&visitor)
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation            *ast.Document
	keepExtensionOrphans bool
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {

	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.ObjectTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		e.operation.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		return
	}

	if e.keepExtensionOrphans {
		return
	}

	e.operation.ImportAndExtendObjectTypeDefinitionByObjectTypeExtension(ref)
}
