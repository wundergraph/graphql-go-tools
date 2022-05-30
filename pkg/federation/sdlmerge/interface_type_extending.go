package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
)

func newExtendInterfaceTypeDefinition(n *normalizer) *extendInterfaceTypeDefinitionVisitor {
	return &extendInterfaceTypeDefinitionVisitor{
		nil,
		nil,
		n,
	}
}

type extendInterfaceTypeDefinitionVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
}

func (e *extendInterfaceTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	e.Walker = walker
	walker.RegisterEnterDocumentVisitor(e)
	walker.RegisterEnterInterfaceTypeExtensionVisitor(e)
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	e.document = operation
}

func (e *extendInterfaceTypeDefinitionVisitor) EnterInterfaceTypeExtension(ref int) {
	nameBytes := e.document.InterfaceTypeExtensionNameBytes(ref)
	nodes, exists := e.document.Index.NodesByNameBytes(nameBytes)
	if !exists {
		return
	}

	hasExtended := false
	isEntity := false
	for _, node := range nodes {
		if node.Kind != ast.NodeKindInterfaceTypeDefinition {
			continue
		}
		if hasExtended {
			if isEntity {
				e.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(string(nameBytes)))
			}
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.InterfaceTypeExtensionNameString(ref)))
		}
		isEntity = e.assessValidEntity(ref, nameBytes)
		e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}

	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InterfaceTypeExtensionNameBytes(ref)))
	}
}

func (e extendInterfaceTypeDefinitionVisitor) getWalker() *astvisitor.Walker {
	return e.Walker
}

func (e extendInterfaceTypeDefinitionVisitor) getDocument() *ast.Document {
	return e.document
}

func (e extendInterfaceTypeDefinitionVisitor) assessValidEntity(ref int, nameBytes []byte) bool {
	extension := e.document.InterfaceTypeExtensions[ref]
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
