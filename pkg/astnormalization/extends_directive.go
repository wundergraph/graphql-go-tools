package astnormalization

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

/*
@extends directive on an object will make it a type extension
type User {
   id: ID!
   name: String
}
     +
type User @key(fields: "id") @extends {
   id: ID! @external
   name: String
}

will be,

type User {
   id: ID!
   name: String
}
*/
type extendsDirectiveVisitor struct {
	operation *ast.Document
}

func extendsDirective(walker *astvisitor.Walker) {
	v := &extendsDirectiveVisitor{}
	walker.RegisterEnterDocumentVisitor(v)
	walker.RegisterEnterObjectTypeDefinitionVisitor(v)
}

func (v *extendsDirectiveVisitor) EnterDocument(operation, _ *ast.Document) {
	v.operation = operation
}

func (v *extendsDirectiveVisitor) EnterObjectTypeDefinition(ref int) {
	if !v.operation.ObjectTypeDefinitions[ref].Directives.HasDirectiveByName(v.operation, "extends") {
		return
	}
	for i := range v.operation.RootNodes {
		if v.operation.RootNodes[i].Ref == ref && v.operation.RootNodes[i].Kind == ast.NodeKindObjectTypeDefinition {
			// give this node a new NodeKind of ObjectTypeExtension
			newRef := v.operation.AddObjectTypeDefinitionExtension(ast.ObjectTypeExtension{ObjectTypeDefinition: v.operation.ObjectTypeDefinitions[ref]})
			// reflect changes inside the root nodes
			v.operation.UpdateRootNode(i, newRef, ast.NodeKindObjectTypeExtension)
			// only remove @extends if the nodes was updated
			v.operation.ObjectTypeExtensions[newRef].Directives.RemoveDirectiveByName(v.operation, "extends")
			break
		}
	}
}
