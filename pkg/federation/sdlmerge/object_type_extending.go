package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendObjectTypeDefinition() *extendObjectTypeDefinitionVisitor {
	return &extendObjectTypeDefinitionVisitor{}
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	operation *ast.Document
}

func (e *extendObjectTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterObjectTypeExtensionVisitor(e)
}

func (e *extendObjectTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.operation = operation
}

func (e *extendObjectTypeDefinitionVisitor) EnterObjectTypeExtension(ref int) {
	nameBytes := e.operation.ObjectTypeExtensionNameBytes(ref)
	nodes, exists := e.operation.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}

	hasExtended := false
	shouldReturn := isQueryOrMutation(nameBytes)
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		if hasExtended {
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.operation.ObjectTypeExtensionNameString(ref)))
		}
		e.operation.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		if shouldReturn {
			return
		}
		hasExtended = true
	}
}

func isQueryOrMutation(nameBytes []byte) bool {
	length := len(nameBytes)
	return isQuery(length, nameBytes) || isMutation(length, nameBytes)
}

func isQuery(length int, b []byte) bool {
	return length == 5 && b[0] == 'Q' && b[1] == 'u' && b[2] == 'e' && b[3] == 'r' && b[4] == 'y'
}

func isMutation(length int, b []byte) bool {
	return length == 8 && b[0] == 'M' && b[1] == 'u' && b[2] == 't' && b[3] == 'a' && b[4] == 't' && b[5] == 'i' && b[6] == 'o' && b[7] == 'n'
}
