// package unsafeparser is for testing purposes only when error handling is overhead and panics are ok
package unsafeparser

import (
	"os"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astparser"
)

func ParseGraphqlDocumentString(input string) ast.Document {
	doc, report := astparser.ParseGraphqlDocumentString(input)
	if report.HasErrors() {
		panic(report.Error())
	}
	return doc
}

func ParseGraphqlDocumentBytes(input []byte) ast.Document {
	doc, report := astparser.ParseGraphqlDocumentBytes(input)
	if report.HasErrors() {
		panic(report.Error())
	}
	return doc
}

func ParseGraphqlDocumentFile(filePath string) ast.Document {
	fileBytes, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	return ParseGraphqlDocumentBytes(fileBytes)
}
