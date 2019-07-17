//go:generate mockgen -source=visitor.go -destination=../mocks/visitor/mock_visitor.go
package astvisitor

import "github.com/jensneuse/graphql-go-tools/pkg/ast"

type Walker struct {
	document *ast.Document
	visitor  Visitor
}

type Visitor interface {
	Enter(kind ast.NodeKind, ref int)
	Leave(kind ast.NodeKind, ref int)
}

func Visit(document *ast.Document, visitor Visitor) {
	walker := Walker{
		document: document,
		visitor:  visitor,
	}
	walker.walk()
}

func (w *Walker) walk() {
	for _, node := range w.document.RootNodes {
		switch node.Kind {
		case ast.NodeKindOperation:
			w.visitOperation(node.Ref)
		}
	}
}

func (w *Walker) visitOperation(ref int) {
	w.visitor.Enter(ast.NodeKindOperation, ref)
	op := w.document.OperationDefinitions[ref]
	w.visitSelectionSet(op.SelectionSet)
	w.visitor.Leave(ast.NodeKindOperation, ref)
}

func (w *Walker) visitSelectionSet(set ast.SelectionSet) {
	w.visitor.Enter(ast.NodeKindSelectionSet, -1)
	for set.Next(w.document) {
		selection, _ := set.Value()
		switch selection.Kind {
		case ast.SelectionKindField:
			w.visitField(selection.Ref)
		}
	}
	w.visitor.Leave(ast.NodeKindSelectionSet, -1)
}

func (w *Walker) visitField(ref int) {
	w.visitor.Enter(ast.NodeKindField, ref)

	field := w.document.Fields[ref]

	if field.SelectionSet.HasNext() {
		w.visitSelectionSet(field.SelectionSet)
	}

	w.visitor.Leave(ast.NodeKindField, ref)
}
