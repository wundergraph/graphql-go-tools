package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func newExtendObjectTypeDefinition() *extendObjectTypeDefinitionVisitor {
	return &extendObjectTypeDefinitionVisitor{}
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func (e *extendObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterObjectTypeExtensionVisitor(e)
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {
	document := e.document
	nameBytes := document.ObjectTypeExtensionNameBytes(ref)
	nodes, exists := document.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}

	hasExtended := false
	shouldReturn := ast.IsRootType(nameBytes)
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(document.ObjectTypeExtensionNameString(ref)))
		}

		document.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		if shouldReturn {
			return
		}
		hasExtended = true
	}

	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(document.ObjectTypeExtensionNameBytes(ref)))
	}
}
