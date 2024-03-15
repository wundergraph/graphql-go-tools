package astnormalization

import (
	"bytes"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/literal"
)

func directiveIncludeSkip(walker *astvisitor.Walker) {
	visitor := directiveIncludeSkipVisitor{
		Walker: walker,
	}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterDirectiveVisitor(&visitor)
}

type directiveIncludeSkipVisitor struct {
	*astvisitor.Walker
	operation, definition *ast.Document
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
	if value.Kind != ast.ValueKindBoolean {
		return
	}
	include := d.operation.BooleanValue(value.Ref)
	switch include {
	case false:
		d.operation.RemoveDirectiveFromNode(d.Ancestors[len(d.Ancestors)-1], ref)
	case true:
		d.handleRemoveNode()
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
	if value.Kind != ast.ValueKindBoolean {
		return
	}
	include := d.operation.BooleanValue(value.Ref)
	switch include {
	case true:
		d.operation.RemoveDirectiveFromNode(d.Ancestors[len(d.Ancestors)-1], ref)
	case false:
		d.handleRemoveNode()
	}
}

func (d *directiveIncludeSkipVisitor) handleRemoveNode() {
	if len(d.Ancestors) < 2 {
		return
	}

	removed := d.operation.RemoveNodeFromSelectionSetNode(d.Ancestors[len(d.Ancestors)-1], d.Ancestors[len(d.Ancestors)-2])
	if !removed {
		return
	}

	if d.Ancestors[len(d.Ancestors)-2].Kind != ast.NodeKindSelectionSet {
		return
	}

	// if the node is the last one, we add a __typename to keep query valid

	selectionSetRef := d.Ancestors[len(d.Ancestors)-2].Ref

	if d.operation.SelectionSetIsEmpty(selectionSetRef) {
		selectionRef, _ := d.typeNameSelection()
		d.operation.AddSelectionRefToSelectionSet(selectionSetRef, selectionRef)
	}
}

func (d *directiveIncludeSkipVisitor) typeNameSelection() (selectionRef int, fieldRef int) {
	field := d.operation.AddField(ast.Field{
		Name: d.operation.Input.AppendInputString("__typename"),
	})
	return d.operation.AddSelectionToDocument(ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	}), field.Ref
}
