package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type collectEntitiesVisitor struct {
	*astvisitor.Walker
	document   *ast.Document
	normalizer *normalizer
}

func newCollectEntitiesVisitor(n *normalizer) *collectEntitiesVisitor {
	return &collectEntitiesVisitor{
		normalizer: n,
	}
}

func (c *collectEntitiesVisitor) Register(walker *astvisitor.Walker) {
	c.Walker = walker
	walker.RegisterEnterDocumentVisitor(c)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(c)
	walker.RegisterEnterObjectTypeDefinitionVisitor(c)
}

func (c *collectEntitiesVisitor) EnterDocument(operation, _ *ast.Document) {
	c.document = operation
}

func (c *collectEntitiesVisitor) EnterInterfaceTypeDefinition(ref int) {
	interfaceType := c.document.InterfaceTypeDefinitions[ref]
	if !interfaceType.HasDirectives {
		return
	}
	name := c.document.InterfaceTypeDefinitionNameString(ref)
	c.resolveEntity(name, interfaceType.Directives.Refs)
}

func (c *collectEntitiesVisitor) EnterObjectTypeDefinition(ref int) {
	objectType := c.document.ObjectTypeDefinitions[ref]
	if !objectType.HasDirectives {
		return
	}
	name := c.document.ObjectTypeDefinitionNameString(ref)
	c.resolveEntity(name, objectType.Directives.Refs)
}

func (c *collectEntitiesVisitor) resolveEntity(name string, directiveRefs []int) {
	entitySet := c.normalizer.entitySet
	if _, exists := entitySet[name]; exists {
		c.Walker.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeDuplicated(name))
	}
	for _, directiveRef := range directiveRefs {
		if c.document.DirectiveNameString(directiveRef) != plan.FederationKeyDirectiveName {
			continue
		}
		entitySet[name] = true
		return
	}
}
