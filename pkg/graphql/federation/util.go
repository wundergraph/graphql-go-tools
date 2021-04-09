package federation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

func isExternalField(document *ast.Document, ref int) bool {
	for _, directiveRef := range document.FieldDefinitions[ref].Directives.Refs {
		if directiveName := document.DirectiveNameString(directiveRef); directiveName == externalDirectiveName {
			return true
		}
	}

	return false
}
