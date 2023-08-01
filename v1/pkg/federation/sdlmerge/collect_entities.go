package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type collectEntitiesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	collectedEntities entitySet
}

func newCollectEntitiesVisitor(collectedEntities entitySet) *collectEntitiesVisitor {
	return &collectEntitiesVisitor{
		collectedEntities: collectedEntities,
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
	name := c.document.InterfaceTypeDefinitionNameString(ref)
	if err := c.resolvePotentialEntity(name, interfaceType.Directives.Refs); err != nil {
		c.StopWithExternalErr(*err)
	}
}

func (c *collectEntitiesVisitor) EnterObjectTypeDefinition(ref int) {
	objectType := c.document.ObjectTypeDefinitions[ref]
	name := c.document.ObjectTypeDefinitionNameString(ref)
	if err := c.resolvePotentialEntity(name, objectType.Directives.Refs); err != nil {
		c.StopWithExternalErr(*err)
	}
}

func (c *collectEntitiesVisitor) resolvePotentialEntity(name string, directiveRefs []int) *operationreport.ExternalError {
	if _, exists := c.collectedEntities[name]; exists {
		err := operationreport.ErrEntitiesMustNotBeDuplicated(name)
		return &err
	}
	for _, directiveRef := range directiveRefs {
		if c.document.DirectiveNameString(directiveRef) != plan.FederationKeyDirectiveName {
			continue
		}
		c.collectedEntities[name] = struct{}{}
		return nil
	}
	return nil
}
