package astnormalization

import (
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// deferIfArgumentType is the type of the "if" argument in the @defer definition of the base schema.
var deferIfArgumentType = []byte("Boolean!")

// deferExpandIntoInternal registers a visitor that
// applies the defer directive to every nested field
func deferExpandIntoInternal(walker *astvisitor.Walker) {
	deferExpandIntoInternalWithDisabled(walker, false)
}

func deferExpandIntoInternalWithDisabled(walker *astvisitor.Walker, disabled bool) *deferExpandIntoInternalVisitor {
	visitor := deferExpandIntoInternalVisitor{
		Walker:  walker,
		disable: disabled,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterInlineFragmentVisitor(&visitor)
	walker.RegisterEnterSelectionSetVisitor(&visitor)

	return &visitor
}

type deferExpandIntoInternalVisitor struct {
	*astvisitor.Walker

	operation      *ast.Document
	defers         []deferInfo
	currentDeferId int

	ignore  bool // external control flag if we should ignore defer per operation
	disable bool // external control flag if we should ignore defer globally
}

type deferInfo struct {
	parentDeferId int
	id            int
	label         string
	fragmentRef   int
}

func (f *deferExpandIntoInternalVisitor) EnterDocument(operation, _ *ast.Document) {
	f.operation = operation
	f.currentDeferId = 0
	f.defers = f.defers[:0]
}

func (f *deferExpandIntoInternalVisitor) EnterInlineFragment(ref int) {
	// expand defer only on operation fields
	// this rule runs after fragments were inlined, but before they removed
	if len(f.Walker.Ancestors) > 0 && f.Walker.Ancestors[0].Kind == ast.NodeKindFragmentDefinition {
		return
	}

	if !f.operation.InlineFragmentHasDirectives(ref) {
		return
	}

	// has defer directive?
	directiveRef, exists := f.operation.InlineFragmentDirectiveByName(ref, literal.DEFER)
	if !exists {
		return
	}

	// An absent variable and a null variable give different results.
	// A cache of normalized operations must use different keys for the two values.
	coercion := f.operation.CoerceIfArgument(directiveRef, f.Ancestors[0].Ref)
	switch coercion {
	case ast.IfArgumentNull:
		ifValue, _ := f.operation.DirectiveArgumentValueByName(directiveRef, literal.IF)
		err := operationreport.ErrNonNullArgumentIsNull(literal.IF, deferIfArgumentType, ifValue.Position)
		// Fragment inlining copies an argument without its position.
		// A zero position is not a valid location, so the error gets no location.
		if ifValue.Position.LineStart == 0 {
			err.Locations = nil
		}
		f.StopWithExternalErr(err)
		return
	case ast.IfArgumentInvalid:
		// Operation validation or variables validation reports the value, so the directive stays in the operation.
		return
	}

	// remove defer directive from the inline fragment
	// as it will be applied to every nested field
	f.operation.RemoveDirectiveFromNode(ast.Node{
		Kind: ast.NodeKindInlineFragment,
		Ref:  ref,
	}, directiveRef)

	if coercion == ast.IfArgumentFalse || f.disable || f.ignore {
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

	// A null label means no label.
	// A caller without the prevalidation rules can pass a label of any value kind.
	labelValue, hasLabel := f.operation.DirectiveArgumentValueByName(directiveRef, literal.LABEL)
	label := ""
	if hasLabel && labelValue.Kind == ast.ValueKindString {
		label = f.operation.StringValueContentString(labelValue.Ref)
	}

	f.currentDeferId++

	parentDeferId := 0
	if len(f.defers) > 0 {
		parentDeferId = f.defers[len(f.defers)-1].id
	}

	deferInfo := deferInfo{
		parentDeferId: parentDeferId,
		id:            f.currentDeferId,
		label:         label,
		fragmentRef:   ref,
	}

	f.defers = append(f.defers, deferInfo)
}

func (f *deferExpandIntoInternalVisitor) LeaveInlineFragment(ref int) {
	if len(f.defers) == 0 {
		return
	}

	if f.defers[len(f.defers)-1].fragmentRef == ref {
		f.defers = f.defers[:len(f.defers)-1]
	}
}

func (f *deferExpandIntoInternalVisitor) EnterSelectionSet(ref int) {
	if len(f.Walker.Ancestors) > 0 && f.Walker.Ancestors[0].Kind == ast.NodeKindFragmentDefinition {
		return
	}

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

func (f *deferExpandIntoInternalVisitor) addInternalDeferDirective(fieldRef int) {
	deferInfo := f.defers[len(f.defers)-1]
	f.operation.AddDeferInternalDirectiveToField(fieldRef, deferInfo.id, deferInfo.label, deferInfo.parentDeferId)
}

func (f *deferExpandIntoInternalVisitor) hasDefers() bool {
	return f.currentDeferId > 0
}
