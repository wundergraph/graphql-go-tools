package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

func newExtendObjectTypeDefinition(n *normalizer) *extendObjectTypeDefinitionVisitor {
	return &extendObjectTypeDefinitionVisitor{
		nil,
		nil,
		n,
	}
}

type extendObjectTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
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
	nameBytes := e.document.ObjectTypeExtensionNameBytes(ref)
	nodes, exists := e.document.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}

	hasExtended := false
	isEntity := false
	shouldReturn := ast.IsRootType(nameBytes)
	for i := range nodes {
		if nodes[i].Kind != ast.NodeKindObjectTypeDefinition {
			continue
		}
		if hasExtended {
			if isEntity {
				e.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(string(nameBytes)))
			}
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(string(nameBytes)))
		}
		isEntity = e.assessValidEntity(ref, nameBytes)
		e.document.ExtendObjectTypeDefinitionByObjectTypeExtension(nodes[i].Ref, ref)
		if shouldReturn {
			return
		}
		hasExtended = true
	}
	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.ObjectTypeExtensionNameBytes(ref)))
	}
}

func (e extendObjectTypeDefinitionVisitor) getWalker() *astvisitor.Walker {
	return e.Walker
}

func (e extendObjectTypeDefinitionVisitor) getDocument() *ast.Document {
	return e.document
}

func (e extendObjectTypeDefinitionVisitor) assessValidEntity(ref int, nameBytes []byte) bool {
	extension := e.document.ObjectTypeExtensions[ref]
	name := string(nameBytes)
	if _, exists := e.normalizer.entities[name]; !exists {
		return false
	}
	if !extension.HasDirectives {
		e.Walker.StopWithExternalErr(operationreport.ErrEntityExtensionMustHaveKeyDirectiveAndExistingPrimaryKey(name))
	}
	primaryKeys := getPrimaryKeys(e, e.normalizer, name, extension.Directives.Refs)
	checkAllPrimaryKeyReferencesAreExternal(e, name, primaryKeys, extension.FieldsDefinition.Refs)
	return true
}
