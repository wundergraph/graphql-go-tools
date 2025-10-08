package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// DeferStreamOnValidOperations validates that defer/stream directives are used on valid operations:
// - Query operations: @defer and @stream are allowed everywhere (root and nested fields)
// - Mutation operations: @defer and @stream are NOT allowed on root fields, but allowed on nested fields
// - Subscription operations: @defer and @stream are NOT allowed anywhere (root or nested fields)
// Directives with if: false are allowed (disabled directives).
// Directives with if: $variable are allowed (dynamic directives that can't be statically determined).
func DeferStreamOnValidOperations() Rule {
	return func(walker *astvisitor.Walker) {
		visitor := deferStreamOnValidOpsVisitor{
			Walker: walker,
		}
		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterOperationVisitor(&visitor)
		walker.RegisterEnterDirectiveVisitor(&visitor)
	}
}

type deferStreamOnValidOpsVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	currentOperationType  ast.OperationType
}

func (d *deferStreamOnValidOpsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *deferStreamOnValidOpsVisitor) EnterOperationDefinition(ref int) {
	d.currentOperationType = d.operation.OperationDefinitions[ref].OperationType
}

func (d *deferStreamOnValidOpsVisitor) EnterDirective(ref int) {
	directiveName := d.operation.DirectiveNameBytes(ref)

	// Only validate @defer and @stream directives
	if !bytes.Equal(directiveName, literal.DEFER) && !bytes.Equal(directiveName, literal.STREAM) {
		return
	}

	if ifValue, hasIf := d.operation.DirectiveArgumentValueByName(ref, literal.IF); hasIf {
		switch ifValue.Kind {
		case ast.ValueKindBoolean:
			// If "if: false", the directive is disabled, so it's allowed
			if !d.operation.BooleanValue(ifValue.Ref) {
				return
			}
		case ast.ValueKindVariable:
			// If if: $variable, we can't statically determine if it's enabled,
			// so we allow it (it might be false at runtime)
			return
		}
	}

	directivePosition := d.operation.Directives[ref].At

	// For subscriptions, @defer and @stream are not allowed anywhere (root or nested)
	if d.currentOperationType == ast.OperationTypeSubscription {
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveNotAllowedOnSubs(
			directiveName,
			directivePosition,
		))
		return
	}

	// For queries, @defer and @stream are allowed everywhere
	if d.currentOperationType == ast.OperationTypeQuery {
		return
	}

	if len(d.Ancestors) == 0 {
		return
	}
	// The directive's immediate parent (the node it's attached to)
	ancestor := d.Ancestors[len(d.Ancestors)-1]

	// Determine if this is a root level directive
	isRootLevel := false

	switch ancestor.Kind {
	case ast.NodeKindInlineFragment:
		// For inline fragments with @defer, check if it's directly in the operation's selection set
		// At root level, ancestors should be: [OperationDefinition, SelectionSet, InlineFragment]
		// For nested: [OperationDefinition, SelectionSet, Field, ..., SelectionSet, InlineFragment]
		if len(d.Ancestors) == 3 {
			// Check if pattern is [OperationDefinition, SelectionSet, InlineFragment]
			if d.Ancestors[0].Kind == ast.NodeKindOperationDefinition &&
				d.Ancestors[1].Kind == ast.NodeKindSelectionSet &&
				d.Ancestors[2].Kind == ast.NodeKindInlineFragment {
				isRootLevel = true
			}
		}
	case ast.NodeKindField:
		// For fields with @stream, check if we're directly in the operation's selection set
		// Count how many SelectionSets we've traversed (depth of nesting)
		// A root-level field has only one SelectionSet ancestor (the operation's selection set)
		selectionSetCount := 0
		for _, a := range d.Ancestors {
			if a.Kind == ast.NodeKindSelectionSet {
				selectionSetCount++
			}
		}
		// If there's only one SelectionSet in the ancestor chain, we're at root level
		isRootLevel = selectionSetCount == 1
	}

	// For mutations, @defer and @stream are not allowed on root fields
	if isRootLevel {
		operationTypeName := d.currentOperationType.Name()
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveNotAllowedOnRootField(
			directiveName,
			operationTypeName,
			directivePosition,
		))
	}
}
