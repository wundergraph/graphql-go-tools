// Package astimport can be used to import Nodes from one ast into another.
//
// This is useful in situations where new ast's should be created from existing ast's.
package astimport

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Importer struct {
	doc    *ast.Document
	parser *astparser.Parser
	report *operationreport.Report
}

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
	i.parser.Prepare(i.doc, i.report)
}

func (i *Importer) ImportType(typeBytes []byte, document *ast.Document) int {
	i.prepare(typeBytes)
	ref := i.parser.ParseType()
	return document.ImportType(ref, i.doc)
}
