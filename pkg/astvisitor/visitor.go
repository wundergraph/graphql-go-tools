//go:generate mockgen -source=visitor.go -destination=../mocks/visitor/mock_visitor.go
package astvisitor

import "github.com/jensneuse/graphql-go-tools/pkg/ast"

type Walker struct {
	document     *ast.Document
	visitor      Visitor
	currentDepth int
	ancestors    []ast.Node
}

type Visitor interface {
	EnterOperationDefinition(ref int, ancestors []ast.Node)
	LeaveOperationDefinition(ref int)
	EnterSelectionSet(set ast.SelectionSet, ancestors []ast.Node)
	LeaveSelectionSet(set ast.SelectionSet, hasNext bool)
	EnterField(ref int, ancestors []ast.Node, hasSelections bool)
	LeaveField(ref int, hasNext bool)
	EnterFragmentSpread(ref int)
	LeaveFragmentSpread(ref int, hasNext bool)
	EnterInlineFragment(ref int)
	LeaveInlineFragment(ref int, hasNext bool)
	EnterFragmentDefinition(ref int)
	LeaveFragmentDefinition(ref int)
}

func Visit(document *ast.Document, visitor Visitor) {
	walker := Walker{
		document:  document,
		visitor:   visitor,
		ancestors: make([]ast.Node, 48)[:0],
	}
	walker.walk()
}

func (w *Walker) walk() {
	for i := range w.document.RootNodes {
		ref := w.document.RootNodes[i].Ref
		switch w.document.RootNodes[i].Kind {
		case ast.NodeKindOperationDefinition:
			w.walkOperationDefinition(ref)
		case ast.NodeKindFragmentDefinition:
			w.walkFragmentDefinition(ref)
		}
	}
}

func (w *Walker) walkOperationDefinition(ref int) {
	w.visitor.EnterOperationDefinition(ref, w.ancestors)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindOperationDefinition, Ref: ref})
	w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet, false)
	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveOperationDefinition(ref)
}

func (w *Walker) walkSelectionSet(set ast.SelectionSet, hasNext bool) {
	w.visitor.EnterSelectionSet(set, w.ancestors)
	for k, i := range set.SelectionRefs {

		hasNext := k+2 <= len(set.SelectionRefs)

		switch w.document.Selections[i].Kind {
		case ast.SelectionKindField:
			w.walkField(w.document.Selections[i].Ref, hasNext)
		case ast.SelectionKindFragmentSpread:
			w.walkFragmentSpread(w.document.Selections[i].Ref, hasNext)
		case ast.SelectionKindInlineFragment:
			w.walkInlineFragment(w.document.Selections[i].Ref, hasNext)
		}
	}
	w.visitor.LeaveSelectionSet(set, hasNext)
}

func (w *Walker) walkField(ref int, hasNext bool) {

	hasSelections := len(w.document.Fields[ref].SelectionSet.SelectionRefs) != 0

	w.visitor.EnterField(ref, w.ancestors, hasSelections)

	if hasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet, hasNext)
	}

	w.visitor.LeaveField(ref, hasNext)
}

func (w *Walker) walkFragmentSpread(ref int, hasNext bool) {
	w.visitor.EnterFragmentSpread(ref)
	w.visitor.LeaveFragmentSpread(ref, hasNext)
}

func (w *Walker) walkInlineFragment(ref int, hasNext bool) {
	w.visitor.EnterInlineFragment(ref)
	if len(w.document.InlineFragments[ref].SelectionSet.SelectionRefs) != 0 {
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet, hasNext)
	}
	w.visitor.LeaveInlineFragment(ref, hasNext)
}

func (w *Walker) walkFragmentDefinition(ref int) {
	w.visitor.EnterFragmentDefinition(ref)

	if len(w.document.FragmentDefinitions[ref].SelectionSet.SelectionRefs) != 0 {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet, false)
	}

	w.visitor.LeaveFragmentDefinition(ref)
}
