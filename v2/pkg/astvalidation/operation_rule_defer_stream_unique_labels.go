package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// DeferStreamHaveUniqueLabels validates that defer and stream directive labels are:
// 1. Unique across all defer and stream directives, whatever the value of the if argument
// 2. Static string values or null, never a variable or another literal kind
func DeferStreamHaveUniqueLabels() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := deferStreamLabelsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type labelPosition struct {
	directiveRef int
	position     position.Position
}

type deferStreamLabelsVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document

	// Track seen labels with their directive refs and positions for duplicate detection.
	seenLabels map[string]labelPosition
}

func (d *deferStreamLabelsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
	// One map covers the document, so a label in a fragment before the operation also reserves its value.
	// With WithRemoveNotMatchingOperationDefinitions, an earlier walk removes the operations that the request did not select.
	d.seenLabels = make(map[string]labelPosition)
}

func (d *deferStreamLabelsVisitor) EnterDirective(ref int) {
	directiveName := d.operation.DirectiveNameBytes(ref)

	if !bytes.Equal(directiveName, literal.DEFER) && !bytes.Equal(directiveName, literal.STREAM) {
		return
	}

	labelValue, hasLabel := d.operation.DirectiveArgumentValueByName(ref, literal.LABEL)
	// A null label means no label, the same as in graphql-js.
	if !hasLabel || labelValue.Kind == ast.ValueKindNull {
		return
	}

	directivePosition := d.operation.Directives[ref].At

	// A variable or a literal that is not a string fails, the same as in graphql-js.
	if labelValue.Kind != ast.ValueKindString {
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveLabelMustBeStatic(directiveName, directivePosition))
		return
	}

	labelString := d.operation.StringValueContentString(labelValue.Ref)

	// The check is static: a label in an unused fragment or in a selection that @skip removes also reserves its value.
	// The walker visits a directive again when a @skip or @include removal changes its selection set.
	if previous, exists := d.seenLabels[labelString]; exists && previous.directiveRef != ref {
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
