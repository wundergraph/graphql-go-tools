package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// DeferStreamDirectiveLabelRule validates that defer and stream directive labels are:
// 1. Unique across all defer and stream directives within an operation
// 2. Not using variables (must be static string values)
func DeferStreamDirectiveLabelRule() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := deferStreamDirectiveLabelVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type labelPosition struct {
	directiveRef int
	position     position.Position
}

type deferStreamDirectiveLabelVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document

	// Track seen labels with their directive refs and positions for duplicate detection.
	seenLabels map[string]labelPosition
}

func (d *deferStreamDirectiveLabelVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *deferStreamDirectiveLabelVisitor) EnterOperationDefinition(ref int) {
	d.seenLabels = make(map[string]labelPosition)
}

func (d *deferStreamDirectiveLabelVisitor) EnterDirective(ref int) {
	directiveName := d.operation.DirectiveNameBytes(ref)

	if !bytes.Equal(directiveName, literal.DEFER) && !bytes.Equal(directiveName, literal.STREAM) {
		return
	}

	labelValue, hasLabel := d.operation.DirectiveArgumentValueByName(ref, literal.LABEL)
	if !hasLabel {
		// No label is okay, directives can be used without labels
		return
	}

	directivePosition := d.operation.Directives[ref].At

	// Labels must be static strings, not variables
	if labelValue.Kind == ast.ValueKindVariable {
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveLabelMustBeStatic(directiveName, directivePosition))
		return
	}

	if labelValue.Kind != ast.ValueKindString {
		// This should be caught by other validation rules, but skip if not a string
		return
	}

	labelString := d.operation.StringValueContentString(labelValue.Ref)

	if previous, exists := d.seenLabels[labelString]; exists {
		previousDirectiveName := d.operation.DirectiveNameBytes(previous.directiveRef)
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveLabelMustBeUnique(
			directiveName,
			previousDirectiveName,
			labelString,
			previous.position,
			directivePosition,
		))
		return
	}

	// Record this label with its position
	d.seenLabels[labelString] = labelPosition{
		directiveRef: ref,
		position:     directivePosition,
	}
}
