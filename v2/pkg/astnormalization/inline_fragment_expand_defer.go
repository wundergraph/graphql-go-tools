package astnormalization

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// inlineFragmentExpandDefer registers a visitor that
// applies the defer directive to every nested field
func inlineFragmentExpandDefer(walker *astvisitor.Walker) {
	visitor := inlineFragmentExpandDeferVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterInlineFragmentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)
}

type inlineFragmentExpandDeferVisitor struct {
	*astvisitor.Walker
	operation      *ast.Document
	defers         []deferInfo
	currentDeferId int
}

type deferInfo struct {
	parentDeferId string
	id            string
	label         string
	fragmentRef   int
}

func (f *inlineFragmentExpandDeferVisitor) EnterDocument(operation, _ *ast.Document) {
	f.operation = operation
}

func (f *inlineFragmentExpandDeferVisitor) EnterInlineFragment(ref int) {
	if !f.operation.InlineFragmentHasDirectives(ref) {
		return
	}

	// has defer directive?
	directiveRef, exists := f.operation.InlineFragmentDirectiveByName(ref, literal.DEFER)
	if !exists {
		return
	}

	// check if defer is enabled
	enabled := true
	ifValue, hasIf := f.operation.DirectiveArgumentValueByName(directiveRef, literal.IF)
	if hasIf {
		enabled = bool(f.operation.BooleanValue(ifValue.Ref))
	}

	// remove defer directive from the inline fragment
	// as it will be applied to every nested field
	f.operation.RemoveDirectiveFromNode(ast.Node{
		Kind: ast.NodeKindInlineFragment,
		Ref:  ref,
	}, directiveRef)

	if !enabled {
		return
	}

	selectionSetRef, ok := f.operation.InlineFragmentSelectionSet(ref)
	if !ok {
		return
	}

	if len(f.operation.SelectionSetFieldSelections(selectionSetRef)) == 0 {
		// if a deferred fragment has no fields, it should be ignored
		return
	}

	// get label argument if any
	labelValue, hasLabel := f.operation.DirectiveArgumentValueByName(directiveRef, literal.LABEL)
	label := ""
	if hasLabel {
		label = f.operation.StringValueContentString(labelValue.Ref)
	}

	f.currentDeferId++

	parentDeferId := ""
	if len(f.defers) > 0 {
		parentDeferId = f.defers[len(f.defers)-1].id
	}

	deferInfo := deferInfo{
		parentDeferId: parentDeferId,
		id:            fmt.Sprintf("%d", f.currentDeferId),
		label:         label,
		fragmentRef:   ref,
	}

	f.defers = append(f.defers, deferInfo)
}

func (f *inlineFragmentExpandDeferVisitor) LeaveInlineFragment(ref int) {
	if len(f.defers) == 0 {
		return
	}

	if f.defers[len(f.defers)-1].fragmentRef == ref {
		f.defers = f.defers[:len(f.defers)-1]
	}
}

func (f *inlineFragmentExpandDeferVisitor) EnterSelectionSet(ref int) {
	// if there are no active defers, nothing to do
	if len(f.defers) == 0 {
		return
	}

	fieldSelectionRefs := f.operation.SelectionSetFieldSelections(ref)
	// if there are no fields in the current selection set, nothing to do
	if len(fieldSelectionRefs) == 0 {
		return
	}

	// apply the internal defer directive to every field in the current selection set
	for _, fieldSelectionRef := range fieldSelectionRefs {
		f.addInternalDeferDirective(f.operation.Selections[fieldSelectionRef].Ref)
	}
}

func (f *inlineFragmentExpandDeferVisitor) addInternalDeferDirective(fieldRef int) {
	var argRefs []int

	deferInfo := f.defers[len(f.defers)-1]

	if deferInfo.id != "" {
		argRefs = append(argRefs, f.addStringArgument("id", deferInfo.id))
	}

	if deferInfo.parentDeferId != "" {
		argRefs = append(argRefs, f.addStringArgument("parentDeferId", deferInfo.parentDeferId))
	}

	if deferInfo.label != "" {
		argRefs = append(argRefs, f.addStringArgument("label", deferInfo.label))
	}

	directive := ast.Directive{
		Name:         f.operation.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: len(argRefs) > 0,
		Arguments: ast.ArgumentList{
			Refs: argRefs,
		},
	}
	directiveRef := f.operation.AddDirective(directive)

	f.operation.AddDirectiveToNode(directiveRef, ast.Node{
		Kind: ast.NodeKindField,
		Ref:  fieldRef,
	})
}

func (f *inlineFragmentExpandDeferVisitor) addStringArgument(name, value string) int {
	strValueRef := f.operation.AddStringValue(ast.StringValue{
		Content: f.operation.Input.AppendInputString(value),
	})

	arg := ast.Argument{
		Name:  f.operation.Input.AppendInputString(name),
		Value: ast.Value{Kind: ast.ValueKindString, Ref: strValueRef},
	}

	return f.operation.AddArgument(arg)
}
