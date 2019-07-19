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
	for i := range w.document.RootNodes {
		switch w.document.RootNodes[i].Kind {
		case ast.NodeKindOperationDefinition:
			w.visitOperation(w.document.RootNodes[i].Ref)
		}
	}
}

func (w *Walker) visitOperation(ref int) {
	w.visitor.Enter(ast.NodeKindOperationDefinition, ref)
	w.visitSelectionSet(w.document.OperationDefinitions[ref].SelectionSet)
	w.visitor.Leave(ast.NodeKindOperationDefinition, ref)
}

func (w *Walker) visitSelectionSet(set ast.SelectionSet) {
	w.visitor.Enter(ast.NodeKindSelectionSet, -1)
	for _, i := range set.SelectionRefs {
		switch w.document.Selections[i].Kind {
		case ast.SelectionKindField:
			w.visitField(w.document.Selections[i].Ref)
		}
	}
	w.visitor.Leave(ast.NodeKindSelectionSet, -1)
}

func (w *Walker) visitField(ref int) {
	w.visitor.Enter(ast.NodeKindField, ref)

	if len(w.document.Fields[ref].SelectionSet.SelectionRefs) != 0 {
		w.visitSelectionSet(w.document.Fields[ref].SelectionSet)
	}

	w.visitor.Leave(ast.NodeKindField, ref)
}
