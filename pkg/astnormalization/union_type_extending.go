package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func extendUnionTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendUnionTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterUnionTypeExtensionVisitor(&visitor)
}

func extendUnionTypeDefinitionKeepingOrphans(walker *astvisitor.Walker) {
	visitor := extendUnionTypeDefinitionVisitor{
		Walker:               walker,
		keepExtensionOrphans: true,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterUnionTypeExtensionVisitor(&visitor)
}

type extendUnionTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation            *ast.Document
	keepExtensionOrphans bool
}

func (e *extendUnionTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendUnionTypeDefinitionVisitor) EnterUnionTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.UnionTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindUnionTypeDefinition {
			continue
		}
		e.operation.ExtendUnionTypeDefinitionByUnionTypeExtension(nodes[i].Ref, ref)
		return
	}

	if e.keepExtensionOrphans {
		return
	}

	e.operation.ImportAndExtendUnionTypeDefinitionByUnionTypeExtension(ref)
}
