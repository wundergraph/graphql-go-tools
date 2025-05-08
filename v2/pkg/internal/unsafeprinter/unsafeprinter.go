package unsafeprinter

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
)

func Print(document *ast.Document) string {
	str, err := astprinter.PrintString(document)
	if err != nil {
		panic(err)
	}
	return str
}

func PrettyPrint(document *ast.Document) string {
	str, err := astprinter.PrintStringIndent(document, "    ")
	if err != nil {
		panic(err)
	}
	return str
}

func Prettify(document string) string {
	doc := unsafeparser.ParseGraphqlDocumentString(document)
	return PrettyPrint(&doc)
}
