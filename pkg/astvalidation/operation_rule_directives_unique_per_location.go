package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

// DirectivesAreUniquePerLocation validates if directives are unique per location
func DirectivesAreUniquePerLocation(repeatableDirectiveNameBytes ...[]byte) Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreUniquePerLocationVisitor{
			Walker:                        walker,
			repeatableDirectivesNameBytes: repeatableDirectiveNameBytes,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreUniquePerLocationVisitor struct {
	*astvisitor.Walker
	operation, definition         *ast.Document
	repeatableDirectivesNameBytes [][]byte
}

func (d *directivesAreUniquePerLocationVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *directivesAreUniquePerLocationVisitor) EnterDirective(ref int) {
	directiveName := d.operation.DirectiveNameBytes(ref)

	for _, repeatable := range d.repeatableDirectivesNameBytes {
		if bytes.Equal(repeatable, directiveName) {
			return
		}
	}

	directives := d.operation.NodeDirectives(d.Ancestors[len(d.Ancestors)-1])

	for _, j := range directives {
		if j == ref {
			continue
		}
		if bytes.Equal(directiveName, d.operation.DirectiveNameBytes(j)) {
			d.StopWithExternalErr(operationreport.ErrDirectiveMustBeUniquePerLocation(directiveName))
			return
		}
	}
}
