package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

// DirectivesAreUniquePerLocation validates if directives are unique per location
func DirectivesAreUniquePerLocation() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := directivesAreUniquePerLocationVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type directivesAreUniquePerLocationVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
	seenDuplicates        map[int]struct{}
}

func (d *directivesAreUniquePerLocationVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
	d.seenDuplicates = make(map[int]struct{})
}

func (d *directivesAreUniquePerLocationVisitor) EnterDirective(ref int) {
	if _, seen := d.seenDuplicates[ref]; seen {
		// skip directive reported as duplicate
		return
	}

	directiveName := d.operation.DirectiveNameBytes(ref)

	directiveDefRef, exists := d.definition.DirectiveDefinitionByNameBytes(directiveName)
	if !exists {
		// ignore unknown directives
		return
	}

	if d.definition.DirectiveDefinitionIsRepeatable(directiveDefRef) {
		// ignore repeatable directives
		return
	}

	nodeDirectives := d.operation.NodeDirectives(d.Ancestors[len(d.Ancestors)-1])
	for _, j := range nodeDirectives {
		if j == ref {
			continue
		}
		if bytes.Equal(directiveName, d.operation.DirectiveNameBytes(j)) {
			d.seenDuplicates[j] = struct{}{}
			d.Report.AddExternalError(operationreport.ErrDirectiveMustBeUniquePerLocation(
				directiveName,
				d.operation.Directives[ref].At,
				d.operation.Directives[j].At,
			))
			return
		}
	}
}
