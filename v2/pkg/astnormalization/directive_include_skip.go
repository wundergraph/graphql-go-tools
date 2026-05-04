package astnormalization

import (
	"bytes"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// directiveIncludeSkip registers a visitor to handle @include and @skip directives.
// It deletes nodes that are evaluated as unused by the directives.
func directiveIncludeSkip(walker *astvisitor.Walker) {
	directiveIncludeSkipKeepNodes(walker, false)
}

// directiveIncludeSkipKeepNodes registers a visitor to handle @include and @skip directives.
// If keepNodes is true, it unconditionally removes the directives and keeps parent nodes.
func directiveIncludeSkipKeepNodes(walker *astvisitor.Walker, keepNodes bool) {
	visitor := directiveIncludeSkipVisitor{
		Walker:    walker,
		keepNodes: keepNodes,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterDirectiveVisitor(&visitor)
}

type directiveIncludeSkipVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	keepNodes             bool
}

func (d *directiveIncludeSkipVisitor) EnterDocument(operation, definition *ast.Document) {
	d.operation = operation
	d.definition = definition
}

func (d *directiveIncludeSkipVisitor) EnterDirective(ref int) {

	name := d.operation.DirectiveNameBytes(ref)

	switch {
	case bytes.Equal(name, literal.INCLUDE):
		d.handleInclude(ref)
	case bytes.Equal(name, literal.SKIP):
		d.handleSkip(ref)
	}
}

func (d *directiveIncludeSkipVisitor) handleSkip(ref int) {
	if len(d.operation.Directives[ref].Arguments.Refs) != 1 {
		return
	}
	arg := d.operation.Directives[ref].Arguments.Refs[0]
	if !bytes.Equal(d.operation.ArgumentNameBytes(arg), literal.IF) {
		return
	}
	value := d.operation.ArgumentValue(arg)
	skip, valid := d.operation.GetBooleanValue(value)
	if !valid {
		return
	}
	if !d.keepNodes && skip {
		d.removeParentNode()
	} else {
		d.operation.RemoveDirectiveFromNode(d.Ancestors[len(d.Ancestors)-1], ref)
	}
}

func (d *directiveIncludeSkipVisitor) handleInclude(ref int) {
	if len(d.operation.Directives[ref].Arguments.Refs) != 1 {
		return
	}
	arg := d.operation.Directives[ref].Arguments.Refs[0]
	if !bytes.Equal(d.operation.ArgumentNameBytes(arg), literal.IF) {
		return
	}
	value := d.operation.ArgumentValue(arg)
	include, valid := d.operation.GetBooleanValue(value)
	if !valid {
		return
	}
	if d.keepNodes || include {
		d.operation.RemoveDirectiveFromNode(d.Ancestors[len(d.Ancestors)-1], ref)
	} else {
		d.removeParentNode()
	}
}

func (d *directiveIncludeSkipVisitor) removeParentNode() {
	if len(d.Ancestors) < 2 {
		return
	}

	parent := d.Ancestors[len(d.Ancestors)-1]
	grandParent := d.Ancestors[len(d.Ancestors)-2]
	removed := d.operation.RemoveNodeFromSelectionSetNode(parent, grandParent)
	if !removed {
		return
	}

	if grandParent.Kind != ast.NodeKindSelectionSet {
		return
	}

	// when we removed a skipped node it could happen that it was the only remaining node in the selection set
	// removing all nodes from the selection set will make query an invalid
	// So we have to add a __typename selection to the selection set,
	// but as this selection was not added by user it should not be added to resolved data

	selectionSetRef := grandParent.Ref

	if d.operation.SelectionSetIsEmpty(selectionSetRef) {
		addInternalTypeNamePlaceholder(d.operation, selectionSetRef)
	}
}

func addInternalTypeNamePlaceholder(operation *ast.Document, selectionSetRef int) int {
	field := operation.AddField(ast.Field{
		Name: operation.Input.AppendInputBytes(literal.TYPENAME),
		// We are adding an alias to the __typename field to mark it as internally added
		// So planner could ignore this field during creation of the response shape
		Alias: ast.Alias{
			IsDefined: true,
			Name:      operation.Input.AppendInputBytes(literal.INTERNAL_TYPENAME),
		},
	})
	selectionRef := operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	})

	operation.AddSelectionRefToSelectionSet(selectionSetRef, selectionRef)
	return field.Ref
}
