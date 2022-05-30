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
		nil,
		nil,
		n,
	}
}

func (c *collectValidEntitiesVisitor) Register(walker *astvisitor.Walker) {
	c.Walker = walker
	c.normalizer.entities = make(map[string]map[string]bool, 0)
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(c)
	walker.RegisterEnterObjectTypeDefinitionVisitor(c)
}

func (c *collectValidEntitiesVisitor) EnterDocument(operation, _ *ast.Document) {
	c.document = operation
}

func (c *collectValidEntitiesVisitor) EnterInterfaceTypeDefinition(ref int) {
	iface := c.document.InterfaceTypeDefinitions[ref]
	if !iface.HasDirectives {
		return
	}

	name := c.document.InterfaceTypeDefinitionNameString(ref)

	if _, exists := c.normalizer.entities[name]; exists {
		c.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(name))
	}

	primaryKeys := c.getPrimaryKeys(name, iface.Directives.Refs)

	if primaryKeys == nil {
		return
	}

	c.checkAllPrimaryKeyReferencesExist(name, iface.FieldsDefinition.Refs, primaryKeys)

	c.normalizer.entities[name] = primaryKeys
}

func (c *collectValidEntitiesVisitor) EnterObjectTypeDefinition(ref int) {
	object := c.document.ObjectTypeDefinitions[ref]
	if !object.HasDirectives {
		return
	}

	name := c.document.ObjectTypeDefinitionNameString(ref)

	if _, exists := c.normalizer.entities[name]; exists {
		c.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(name))
	}

	primaryKeys := c.getPrimaryKeys(name, object.Directives.Refs)

	c.checkAllPrimaryKeyReferencesExist(name, object.FieldsDefinition.Refs, primaryKeys)
}

func (c *collectValidEntitiesVisitor) getPrimaryKeys(name string, directiveRefs []int) map[string]bool {
	primaryKeys := make(map[string]bool, 0)
	for _, directiveRef := range directiveRefs {
		if c.document.DirectiveNameString(directiveRef) != "key" {
			continue
		}
		directive := c.document.Directives[directiveRef]
		if len(directive.Arguments.Refs) > 1 {
			c.Walker.StopWithExternalErr(operationreport.ErrKeyDirectiveMustHaveSingleArgument(name))
		}
		argumentRef := directive.Arguments.Refs[0]
		primaryKey := c.document.StringValueContentString(c.document.Arguments[argumentRef].Value.Ref)
		if primaryKey == "" {
			c.Walker.StopWithExternalErr(operationreport.ErrPrimaryKeyReferencesMustExistOnEntity(primaryKey, name))
		}
		primaryKeys[primaryKey] = false
	}
	return primaryKeys
}

func (c *collectValidEntitiesVisitor) checkAllPrimaryKeyReferencesExist(name string, fieldRefs []int, primaryKeys map[string]bool) {
	fieldReferences := len(primaryKeys)
	if fieldReferences < 1 {
		return
	}
	for _, fieldRef := range fieldRefs {
		fieldName := c.document.FieldDefinitionNameString(fieldRef)
		isResolved, isPrimaryKey := primaryKeys[fieldName]
		if !isPrimaryKey {
			continue
		}
		if !isResolved {
			primaryKeys[fieldName] = true
			fieldReferences -= 1
		}

		if fieldReferences == 0 {
			c.normalizer.entities[name] = primaryKeys
			return
		}
	}
	for primaryKey, isResolved := range primaryKeys {
		if !isResolved {
			c.Walker.StopWithExternalErr(operationreport.ErrPrimaryKeyReferencesMustExistOnEntity(primaryKey, name))
		}
	}
}
