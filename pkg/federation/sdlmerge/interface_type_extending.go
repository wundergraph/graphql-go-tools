package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
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
	e.normalizer.entityValidator.setDocument(operation)
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
				e.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeDuplicated(string(nameBytes)))
			}
			e.Walker.StopWithExternalErr(operationreport.ErrSharedTypesMustNotBeExtended(e.document.InterfaceTypeExtensionNameString(ref)))
		}
		isEntity = e.isEntity(ref, nameBytes)
		e.document.ExtendInterfaceTypeDefinitionByInterfaceTypeExtension(node.Ref, ref)
		hasExtended = true
	}
	if !hasExtended {
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionOrphansMustResolveInSupergraph(e.document.InterfaceTypeExtensionNameBytes(ref)))
	}
}

func (e *extendInterfaceTypeDefinitionVisitor) isEntity(ref int, nameBytes []byte) bool {
	extension := e.document.InterfaceTypeExtensions[ref]
	validator := e.normalizer.entityValidator
	name := string(nameBytes)
	if _, exists := validator.entitySet[name]; !exists {
		if !extension.HasDirectives || !validator.isEntityExtension(extension.Directives.Refs) {
			return false
		}
		e.Walker.StopWithExternalErr(operationreport.ErrExtensionWithKeyDirectiveMustExtendEntity(name))
	}
	if !extension.HasDirectives {
		e.Walker.StopWithExternalErr(operationreport.ErrEntityExtensionMustHaveKeyDirective(name))
	}
	primaryKeys, err := validator.getPrimaryKeys(name, extension.Directives.Refs, true)
	if err != nil {
		e.Walker.StopWithExternalErr(*err)
	}
	err = validator.validateExternalPrimaryKeys(name, primaryKeys, extension.FieldsDefinition.Refs)
	if err != nil {
		e.Walker.StopWithExternalErr(*err)
	}
	return true
}
