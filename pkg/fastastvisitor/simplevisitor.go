package fastastvisitor

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
)

type SimpleWalker struct {
	err              error
	document         *ast.Document
	definition       *ast.Document
	depth            int
	Ancestors        []ast.Node
	parentDefinition ast.Node
	visitor          AllNodesVisitor
}

func NewSimpleWalker(ancestorSize int) SimpleWalker {
	return SimpleWalker{
		Ancestors: make([]ast.Node, 0, ancestorSize),
	}
}

func (w *SimpleWalker) SetVisitor(visitor AllNodesVisitor) {
	w.visitor = visitor
}

func (w *SimpleWalker) Walk(document, definition *ast.Document) error {

	if w.visitor == nil {
		return fmt.Errorf("visitor must not be nil, use SetVisitor()")
	}

	w.err = nil
	w.Ancestors = w.Ancestors[:0]
	w.document = document
	w.definition = definition
	w.depth = 0
	w.walk()
	return w.err
}

func (w *SimpleWalker) appendAncestor(ref int, kind ast.NodeKind) {
	w.Ancestors = append(w.Ancestors, ast.Node{
		Kind: kind,
		Ref:  ref,
	})
}

func (w *SimpleWalker) removeLastAncestor() {
	w.Ancestors = w.Ancestors[:len(w.Ancestors)-1]
}

func (w *SimpleWalker) increaseDepth() {
	w.depth++
}

func (w *SimpleWalker) decreaseDepth() {
	w.depth--
}

func (w *SimpleWalker) walk() {

	if w.document == nil {
		w.err = fmt.Errorf("document must not be nil")
		return
	}

	w.visitor.EnterDocument(w.document, w.definition)

	for i := range w.document.RootNodes {
		isLast := i == len(w.document.RootNodes)-1
		ref := w.document.RootNodes[i].Ref
		switch w.document.RootNodes[i].Kind {
		case ast.NodeKindOperationDefinition:
			if w.definition == nil {
				w.err = fmt.Errorf("definition must not be nil when walking operations")
				return
			}
			w.walkOperationDefinition(ref, isLast)
		case ast.NodeKindFragmentDefinition:
			if w.definition == nil {
				w.err = fmt.Errorf("definition must not be nil when walking operations")
				return
			}
			w.walkFragmentDefinition(ref)
		}
	}

	w.visitor.LeaveDocument(w.document, w.definition)
}

func (w *SimpleWalker) walkOperationDefinition(ref int, isLastRootNode bool) {
	w.increaseDepth()

	w.visitor.EnterOperationDefinition(ref)

	w.appendAncestor(ref, ast.NodeKindOperationDefinition)

	if w.document.OperationDefinitions[ref].HasVariableDefinitions {
		for _, i := range w.document.OperationDefinitions[ref].VariableDefinitions.Refs {
			w.walkVariableDefinition(i)
		}
	}

	if w.document.OperationDefinitions[ref].HasDirectives {
		for _, i := range w.document.OperationDefinitions[ref].Directives.Refs {
			w.walkDirective(i)
		}
	}

	if w.document.OperationDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.visitor.LeaveOperationDefinition(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkVariableDefinition(ref int) {
	w.increaseDepth()

	w.visitor.EnterVariableDefinition(ref)

	w.appendAncestor(ref, ast.NodeKindVariableDefinition)

	if w.document.VariableDefinitions[ref].HasDirectives {
		for _, i := range w.document.VariableDefinitions[ref].Directives.Refs {
			w.walkDirective(i)
		}
	}

	w.removeLastAncestor()

	w.visitor.LeaveVariableDefinition(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkSelectionSet(ref int) {
	w.increaseDepth()

	w.visitor.EnterSelectionSet(ref)

	w.appendAncestor(ref, ast.NodeKindSelectionSet)

	for _, j := range w.document.SelectionSets[ref].SelectionRefs {

		switch w.document.Selections[j].Kind {
		case ast.SelectionKindField:
			w.walkField(w.document.Selections[j].Ref)
		case ast.SelectionKindFragmentSpread:
			w.walkFragmentSpread(w.document.Selections[j].Ref)
		case ast.SelectionKindInlineFragment:
			w.walkInlineFragment(w.document.Selections[j].Ref)
		}
	}

	w.removeLastAncestor()

	w.visitor.LeaveSelectionSet(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkField(ref int) {
	w.increaseDepth()

	w.visitor.EnterField(ref)

	w.appendAncestor(ref, ast.NodeKindField)

	if len(w.document.Fields[ref].Arguments.Refs) != 0 {
		for _, i := range w.document.Fields[ref].Arguments.Refs {
			w.walkArgument(i)
		}
	}

	if w.document.Fields[ref].HasDirectives {
		for _, i := range w.document.Fields[ref].Directives.Refs {
			w.walkDirective(i)
		}
	}

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.visitor.LeaveField(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkDirective(ref int) {
	w.increaseDepth()

	w.visitor.EnterDirective(ref)

	w.appendAncestor(ref, ast.NodeKindDirective)

	if w.document.Directives[ref].HasArguments {
		for _, i := range w.document.Directives[ref].Arguments.Refs {
			w.walkArgument(i)
		}
	}

	w.removeLastAncestor()

	w.visitor.LeaveDirective(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkArgument(ref int) {
	w.increaseDepth()

	w.visitor.EnterArgument(ref)
	w.visitor.LeaveArgument(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkFragmentSpread(ref int) {
	w.increaseDepth()

	w.visitor.EnterFragmentSpread(ref)
	w.visitor.LeaveFragmentSpread(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) walkInlineFragment(ref int) {
	w.increaseDepth()

	w.visitor.EnterInlineFragment(ref)

	w.appendAncestor(ref, ast.NodeKindInlineFragment)

	if w.document.InlineFragments[ref].HasDirectives {
		for _, i := range w.document.InlineFragments[ref].Directives.Refs {
			w.walkDirective(i)
		}
	}

	if w.document.InlineFragments[ref].HasSelections {
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.visitor.LeaveInlineFragment(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) inlineFragmentTypeDefinition(ref int, enclosingTypeDefinition ast.Node) ast.Node {
	typeRef := w.document.InlineFragments[ref].TypeCondition.Type
	if typeRef == -1 {
		return enclosingTypeDefinition
	}
	typeCondition := w.document.Types[w.document.InlineFragments[ref].TypeCondition.Type]
	return w.definition.Index.Nodes[string(w.document.Input.ByteSlice(typeCondition.Name))]
}

func (w *SimpleWalker) walkFragmentDefinition(ref int) {
	w.increaseDepth()

	w.visitor.EnterFragmentDefinition(ref)

	w.appendAncestor(ref, ast.NodeKindFragmentDefinition)

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet)
	}

	w.removeLastAncestor()

	w.visitor.LeaveFragmentDefinition(ref)

	w.decreaseDepth()
}

func (w *SimpleWalker) refsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
