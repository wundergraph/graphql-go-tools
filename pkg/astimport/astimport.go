// Package astimport can be used to import Nodes manually into an AST.
//
// This is useful when an AST should be created manually.
package astimport

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

// Importer imports Nodes into an existing AST.
// Always use NewImporter() to create a new Importer.
type Importer struct {
	doc    *ast.Document
	parser *astparser.Parser
	report *operationreport.Report
}

// NewImporter creates a default Importer
func NewImporter() *Importer {
	return &Importer{
		parser: astparser.NewParser(),
		doc:    ast.NewDocument(),
		report: &operationreport.Report{},
	}
}

func (i *Importer) prepare (inputBytes []byte) {
	i.doc.Reset()
	i.report.Reset()
	i.doc.Input.ResetInputBytes(inputBytes)
	i.parser.PrepareImport(i.doc, i.report)
}

// ImportType imports a Type in GraphQL format into the provided AST.
func (i *Importer) ImportType(typeBytes []byte, document *ast.Document) int {
	i.prepare(typeBytes)
	ref := i.parser.ParseType()
	return document.ImportType(ref, i.doc)
}
