package planconfiguration

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

const (
	keyDirectiveName      = "key"
	requireDirectiveName  = "requires"
	externalDirectiveName = "external"
)

func isExternalField(document *ast.Document, ref int) bool {
	for _, directiveRef := range document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName == externalDirectiveName {
			return true
		}
	}

	return false
}

func isEntity(document *ast.Document, objectType ast.ObjectTypeDefinition) bool {
	for _, directiveRef := range objectType.Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName == keyDirectiveName {
			return true
		}
	}

	return false
}
