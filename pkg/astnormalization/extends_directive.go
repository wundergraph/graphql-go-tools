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
	document *ast.Document
}

func extendsDirective(walker *astvisitor.Walker) {
	v := &extendsDirectiveVisitor{}
	walker.RegisterEnterDocumentVisitor(v)
	walker.RegisterEnterObjectTypeDefinitionVisitor(v)
}

func (v *extendsDirectiveVisitor) EnterDocument(document, _ *ast.Document) {
	v.document = document
}

func (v *extendsDirectiveVisitor) EnterObjectTypeDefinition(ref int) {
	if !v.document.ObjectTypeDefinitions[ref].Directives.HasDirectiveByName(v.document, "extends") {
		return
	}
	for i := range v.document.RootNodes {
		if v.document.RootNodes[i].Ref == ref && v.document.RootNodes[i].Kind == ast.NodeKindObjectTypeDefinition {
			// give this node a new NodeKind of ObjectTypeExtension
			newRef := v.document.AddObjectTypeDefinitionExtension(ast.ObjectTypeExtension{ObjectTypeDefinition: v.document.ObjectTypeDefinitions[ref]})
			// reflect changes inside the root nodes
			v.document.UpdateRootNode(i, newRef, ast.NodeKindObjectTypeExtension)
			// only remove @extends if the nodes was updated
			v.document.ObjectTypeExtensions[newRef].Directives.RemoveDirectiveByName(v.document, "extends")
			break
		}
	}
}
