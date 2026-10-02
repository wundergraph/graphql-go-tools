package astvalidation

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// DeferStreamOnValidOperations rejects @defer and @stream on a selection whose parent type is the mutation or subscription root type, as graphql-js does.
// In a subscription the rule also rejects a nested directive that the if argument enables.
// Run the rule as a prevalidation rule, because normalization removes a disabled @defer before full validation.
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
	currentOperationRef   int
}

func (d *deferStreamOnValidOpsVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
	// The walker reuses the visitor, and a fragment definition can come before the first operation.
	d.currentOperationType = ast.OperationTypeUnknown
	d.currentOperationRef = -1
}

func (d *deferStreamOnValidOpsVisitor) EnterOperationDefinition(ref int) {
	d.currentOperationType = d.operation.OperationDefinitions[ref].OperationType
	d.currentOperationRef = ref
}

func (d *deferStreamOnValidOpsVisitor) EnterDirective(ref int) {
	directiveName := d.operation.DirectiveNameBytes(ref)
	if !bytes.Equal(directiveName, literal.DEFER) && !bytes.Equal(directiveName, literal.STREAM) {
		return
	}

	directivePosition := d.operation.Directives[ref].At

	// The walker can visit a fragment definition before the operation, so the message does not read currentOperationType.
	if d.isRootSelection(d.definition.Index.MutationTypeName) {
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveNotAllowedOnRootField(
			directiveName,
			ast.OperationTypeMutation.Name(),
			directivePosition,
		))
		return
	}

	if d.isRootSelection(d.definition.Index.SubscriptionTypeName) ||
		(d.currentOperationType == ast.OperationTypeSubscription &&
			d.operation.CoerceIfArgument(ref, d.currentOperationRef) == ast.IfArgumentTrue) {
		d.StopWithExternalErr(operationreport.ErrDeferStreamDirectiveNotAllowedOnSubs(
			directiveName,
			directivePosition,
		))
	}
}

// isRootSelection reports whether the parent type of the directive location is the root type.
// The walker pushes the type of a field or of a typed inline fragment before it walks the directives of that node.
// For these nodes the parent type is one entry lower in TypeDefinitions.
func (d *deferStreamOnValidOpsVisitor) isRootSelection(rootTypeName ast.ByteSlice) bool {
	if len(d.Ancestors) == 0 {
		return false
	}

	typeDefinitions := d.TypeDefinitions
	switch location := d.Ancestors[len(d.Ancestors)-1]; location.Kind {
	case ast.NodeKindField:
		typeDefinitions = typeDefinitions[:len(typeDefinitions)-1]
	case ast.NodeKindInlineFragment:
		if d.operation.InlineFragmentHasTypeCondition(location.Ref) {
			typeDefinitions = typeDefinitions[:len(typeDefinitions)-1]
		}
	}
	if len(typeDefinitions) == 0 {
		return false
	}

	parent := typeDefinitions[len(typeDefinitions)-1]
	// An unknown field pushes an invalid node with an empty name.
	// A schema without the root type has an empty root type name.
	// Only an object type definition can be a root type, so an invalid parent does not match an empty root type name.
	if parent.Kind != ast.NodeKindObjectTypeDefinition {
		return false
	}
	return bytes.Equal(d.definition.NodeNameBytes(parent), rootTypeName)
}
