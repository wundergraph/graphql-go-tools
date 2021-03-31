package federation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"os"
	"strings"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
)

func BuildBaseSchemaFromSDLs(SDLs ...string) (string, error) {
	SDL := strings.Join(SDLs, "\n")

	doc, report := astparser.ParseGraphqlDocumentString(SDL)
	if report.HasErrors() {
		return "", report
	}

	normalizer := astnormalization.NewNormalizer(true, true)
	normalizer.NormalizeOperation(&doc, nil, &report)
	if report.HasErrors() {
		return "", report
	}

	if err := printSchema(&doc, "schema2.graphql"); err != nil {
		return "", err
	}

	if err := prettifySchema(&doc, "pretty_ast.txt"); err != nil {
		return "", err
	}

	return "", nil
}

func printSchema(doc *ast.Document, fileName string) error {
	fi, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer func() { _ = fi.Close() }()

	if err := astprinter.PrintIndent(doc, nil, []byte("  "), fi); err != nil {
		return err
	}

	return nil
}

func prettifySchema(doc *ast.Document, fileName string) error {
	report := operationreport.Report{}

	fi, err := os.Create(fileName)
	if err != nil {
		return err
	}

	defer func() { _ = fi.Close() }()

	walker := astvisitor.NewWalker(48)
	visitor := &printingVisitor{
		Walker:     &walker,
		out:        fi,
		operation:  doc,
		definition: doc,
	}

	walker.RegisterAllNodesVisitor(visitor)

	walker.Walk(doc, doc, &report)

	if report.HasErrors() {
		return report
	}

	return nil
}

const baseFederationSchema = `
scalar _Any
scalar _FieldSet

directive @external on FIELD_DEFINITION
directive @requires(fields: _FieldSet!) on FIELD_DEFINITION
directive @provides(fields: _FieldSet!) on FIELD_DEFINITION
directive @key(fields: _FieldSet!) on OBJECT | INTERFACE
directive @extends on OBJECT | INTERFACE
`
