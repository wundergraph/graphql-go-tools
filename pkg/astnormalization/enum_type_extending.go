package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func extendEnumTypeDefinition(walker *astvisitor.Walker) {
	visitor := extendEnumTypeDefinitionVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterEnumTypeExtensionVisitor(&visitor)
}

func extendEnumTypeDefinitionKeepingOrphans(walker *astvisitor.Walker) {
	visitor := extendEnumTypeDefinitionVisitor{
		Walker:               walker,
		keepExtensionOrphans: true,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterEnumTypeExtensionVisitor(&visitor)
}

type extendEnumTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation            *ast.Document
	keepExtensionOrphans bool
}

func (e *extendEnumTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendEnumTypeDefinitionVisitor) EnterEnumTypeExtension(ref int) {
	nodes, exists := e.operation.Index.NodesByNameBytes(e.operation.EnumTypeExtensionNameBytes(ref))
	if !exists {
		return
	}

	isOrphan := true
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindEnumTypeDefinition {
			continue
		}
		isOrphan = false
		e.operation.ExtendEnumTypeDefinitionByEnumTypeExtension(nodes[i].Ref, ref)
		return
	}

	if e.keepExtensionOrphans && isOrphan {
		return
	}

	e.operation.ImportAndExtendEnumTypeDefinitionByEnumTypeExtension(ref)
}
