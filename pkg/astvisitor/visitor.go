//go:generate mockgen -source=visitor.go -destination=../mocks/visitor/mock_visitor.go
package astvisitor

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
)

type Walker struct {
	document     *ast.Document
	input        *input.Input
	visitor      Visitor
	currentDepth int
	ancestors    []ast.Node
}

type Visitor interface {
	EnterOperationDefinition(ref int)
	LeaveOperationDefinition(ref int)
	EnterSelectionSet(ref int, ancestors []ast.Node)
	LeaveSelectionSet(ref int)
	EnterField(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool)
	LeaveField(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool)
	EnterFragmentSpread(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int)
	LeaveFragmentSpread(ref int)
	EnterInlineFragment(ref int, ancestors []ast.Node, selectionSet int, selectionsBefore []int, selectionsAfter []int, hasSelections bool)
	LeaveInlineFragment(ref int)
	EnterFragmentDefinition(ref int)
	LeaveFragmentDefinition(ref int)
}

func (w *Walker) Visit(document *ast.Document, input *input.Input, visitor Visitor) {
	w.ancestors = w.ancestors[:0]
	w.document = document
	w.input = input
	w.visitor = visitor
	w.walk()
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
	w.visitor.EnterOperationDefinition(ref)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindOperationDefinition, Ref: ref})

	w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet)

	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveOperationDefinition(ref)
}

func (w *Walker) walkSelectionSet(ref int) {
	w.visitor.EnterSelectionSet(ref, w.ancestors)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindSelectionSet, Ref: ref})

	for i, j := range w.document.SelectionSets[ref].SelectionRefs {

		selectionsBefore := w.document.SelectionSets[ref].SelectionRefs[:i]
		selectionsAfter := w.document.SelectionSets[ref].SelectionRefs[i+1:]

		switch w.document.Selections[j].Kind {
		case ast.SelectionKindField:
			w.walkField(w.document.Selections[j].Ref, ref, selectionsBefore, selectionsAfter)
		case ast.SelectionKindFragmentSpread:
			w.walkFragmentSpread(w.document.Selections[j].Ref, ref, selectionsBefore, selectionsAfter)
		case ast.SelectionKindInlineFragment:
			w.walkInlineFragment(w.document.Selections[j].Ref, ref, selectionsBefore, selectionsAfter)
		}
	}

	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveSelectionSet(ref)
}

func (w *Walker) walkField(ref int, selectionSet int, selectionsBefore, selectionsAfter []int) {

	w.visitor.EnterField(ref, w.ancestors, selectionSet, selectionsBefore, selectionsAfter, w.document.Fields[ref].HasSelections)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindField, Ref: ref})

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet)
	}

	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveField(ref, w.ancestors, selectionSet, selectionsBefore, selectionsAfter, w.document.Fields[ref].HasSelections)
}

func (w *Walker) walkFragmentSpread(ref int, selectionSet int, selectionsBefore, selectionsAfter []int) {
	w.visitor.EnterFragmentSpread(ref, w.ancestors, selectionSet, selectionsBefore, selectionsAfter)
	w.visitor.LeaveFragmentSpread(ref)
}

func (w *Walker) walkInlineFragment(ref int, selectionSet int, selectionsBefore, selectionsAfter []int) {
	w.visitor.EnterInlineFragment(ref, w.ancestors, selectionSet, selectionsBefore, selectionsAfter, w.document.InlineFragments[ref].HasSelections)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindInlineFragment, Ref: ref})

	if w.document.InlineFragments[ref].HasSelections {
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet)
	}

	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveInlineFragment(ref)
}

func (w *Walker) walkFragmentDefinition(ref int) {
	w.visitor.EnterFragmentDefinition(ref)
	w.ancestors = append(w.ancestors, ast.Node{Kind: ast.NodeKindFragmentDefinition, Ref: ref})

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet)
	}

	w.ancestors = w.ancestors[:len(w.ancestors)-1]
	w.visitor.LeaveFragmentDefinition(ref)
}
