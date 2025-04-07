package plan

import (
	"fmt"
	"slices"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type deferVisitor struct {
	walker    *astvisitor.Walker
	operation *ast.Document

	deferredFragments     []resolve.DeferInfo
	deferredFragmentStack []resolve.DeferInfo
	deferredFields        map[int]resolve.DeferInfo
}

var _ astvisitor.EnterDocumentVisitor = (*deferVisitor)(nil)
var _ astvisitor.InlineFragmentVisitor = (*deferVisitor)(nil)
var _ astvisitor.FragmentSpreadVisitor = (*deferVisitor)(nil)
var _ astvisitor.EnterFieldVisitor = (*deferVisitor)(nil)

var errDuplicateDefer = fmt.Errorf("duplicate defer")

func (v *deferVisitor) EnterDocument(operation, definition *ast.Document) {
	v.operation = operation
	v.deferredFragments = nil
	v.deferredFragmentStack = nil
	v.deferredFields = make(map[int]resolve.DeferInfo)
}

func (v *deferVisitor) EnterInlineFragment(ref int) {
	directives := v.operation.InlineFragments[ref].Directives.Refs
	if _, ok := v.operation.DirectiveWithNameBytes(directives, literal.DEFER); ok {
		v.enterDefer(ast.PathItem{
			Kind:        ast.InlineFragmentName,
			FieldName:   v.operation.InlineFragmentTypeConditionName(ref),
			FragmentRef: v.walker.CurrentRef,
		})
	}
}

func (v *deferVisitor) LeaveInlineFragment(ref int) {
	directives := v.operation.InlineFragments[ref].Directives.Refs
	if _, ok := v.operation.DirectiveWithNameBytes(directives, literal.DEFER); ok {
		v.leaveDefer()
	}
}

func (v *deferVisitor) EnterFragmentSpread(ref int) {
	// TODO(cd): Fragment spreads are expanded to inline fragments during normalization. Skipping these.
}

func (v *deferVisitor) LeaveFragmentSpread(ref int) {
}

func (v *deferVisitor) EnterField(ref int) {
	if v.inDefer() {
		v.deferredFields[ref] = v.currentDefer()
	}
}

func (v *deferVisitor) enterDefer(item ast.PathItem) {
	fullPath := v.fullPathFor(item)

	info := resolve.DeferInfo{Path: fullPath}

	if slices.ContainsFunc(v.deferredFragments, func(el resolve.DeferInfo) bool {
		return el.Equals(&info)
	}) {
		v.walker.StopWithInternalErr(fmt.Errorf("%w for %s %d", errDuplicateDefer, v.walker.CurrentKind.String(), v.walker.CurrentRef))
		return
	}

	// v.deferredFragmentStack = append(v.deferredFragmentStack, info)
	v.deferredFragments = append(v.deferredFragments, info)
	v.deferredFragmentStack = append(v.deferredFragmentStack, info)
}

func (v *deferVisitor) leaveDefer() {
	if !v.inDefer() {
		return
	}
	v.deferredFragmentStack = v.deferredFragmentStack[:len(v.deferredFragmentStack)-1]
}

func (v *deferVisitor) inDefer() bool {
	return len(v.deferredFragmentStack) > 0
}

func (v *deferVisitor) currentDefer() resolve.DeferInfo {
	return v.deferredFragmentStack[len(v.deferredFragmentStack)-1]
}

func (v *deferVisitor) fullPathFor(item ast.PathItem) ast.Path {
	fullPath := append(make([]ast.PathItem, 0, len(v.walker.Path)+1), v.walker.Path...)
	fullPath = append(fullPath, item)

	return fullPath
}
