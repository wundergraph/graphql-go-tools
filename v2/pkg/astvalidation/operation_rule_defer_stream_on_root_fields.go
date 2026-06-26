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
		// The directive is only enabled and checked when the value resolves to true.
		// This mirrors how inlineDefer works.
		if enabled, ok := d.operation.GetBooleanValue(ifValue); !ok || !enabled {
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

	// For mutations, @defer/@stream are only disallowed on root fields.
	selectionSetCount := 0
	for _, a := range d.Ancestors {
		if a.Kind == ast.NodeKindSelectionSet {
			selectionSetCount++
		}
	}
	// More than one selection set means the directive is nested below a field.
	if selectionSetCount != 1 {
		return
	}

	isRootLevel := false
	switch root := d.Ancestors[0]; root.Kind {
	case ast.NodeKindOperationDefinition:
		// Directly inside the (mutation) operation's selection set.
		isRootLevel = true
	case ast.NodeKindFragmentDefinition:
		// A fragment defined on the mutation root type contributes the
		// operation's root fields when spread at the operation root.
		typeName := d.operation.FragmentDefinitionTypeName(root.Ref)
		isRootLevel = bytes.Equal(typeName, d.definition.Index.MutationTypeName)
	}

	if isRootLevel {
		operationTypeName := d.currentOperationType.Name()
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveNotAllowedOnRootField(
			directiveName,
			operationTypeName,
			directivePosition,
		))
	}
}
