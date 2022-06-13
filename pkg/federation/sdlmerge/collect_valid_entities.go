package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type collectValidEntitiesVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
}

func newCollectValidEntitiesVisitor(n *normalizer) *collectValidEntitiesVisitor {
	return &collectValidEntitiesVisitor{
		normalizer: n,
	}
}

func (c *collectValidEntitiesVisitor) Register(walker *astvisitor.Walker) {
	c.Walker = walker
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(c)
	walker.RegisterEnterObjectTypeDefinitionVisitor(c)
}

func (c *collectValidEntitiesVisitor) EnterDocument(operation, _ *ast.Document) {
	c.document = operation
	c.normalizer.entityValidator.setDocument(operation)
}

func (c *collectValidEntitiesVisitor) EnterInterfaceTypeDefinition(ref int) {
	interfaceType := c.document.InterfaceTypeDefinitions[ref]
	if !interfaceType.HasDirectives {
		return
	}
	name := c.document.InterfaceTypeDefinitionNameString(ref)
	c.resolveEntity(name, interfaceType.Directives.Refs, interfaceType.FieldsDefinition.Refs)
}

func (c *collectValidEntitiesVisitor) EnterObjectTypeDefinition(ref int) {
	objectType := c.document.ObjectTypeDefinitions[ref]
	if !objectType.HasDirectives {
		return
	}
	name := c.document.ObjectTypeDefinitionNameString(ref)
	c.resolveEntity(name, objectType.Directives.Refs, objectType.FieldsDefinition.Refs)
}

func (c *collectValidEntitiesVisitor) resolveEntity(name string, directiveRefs []int, fieldRefs []int) {
	validator := c.normalizer.entityValidator
	if _, exists := validator.entitySet[name]; exists {
		c.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeDuplicated(name))
	}
	primaryKeys, err := validator.getPrimaryKeys(name, directiveRefs)
	if err != nil {
		c.Walker.StopWithExternalErr(*err)
	}
	if primaryKeys == nil {
		return
	}
	validator.entitySet[name] = primaryKeys
	err = validator.validatePrimaryKeyReferences(name, fieldRefs)
	if err != nil {
		c.Walker.StopWithExternalErr(*err)
	}
}
