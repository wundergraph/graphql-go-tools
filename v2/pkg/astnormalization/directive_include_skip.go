package astnormalization

import (
	"bytes"

	"github.com/buger/jsonparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
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
	var skip ast.BooleanValue
	switch value.Kind {
	case ast.ValueKindBoolean:
		skip = d.operation.BooleanValue(value.Ref)
	case ast.ValueKindVariable:
		val, valid := d.getVariableValue(d.operation.VariableValueNameString(value.Ref))
		if !valid {
			return
		}
		skip = ast.BooleanValue(val)
	default:
		return
	}
	if skip {
		d.handleRemoveNode()
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
	var include ast.BooleanValue
	switch value.Kind {
	case ast.ValueKindBoolean:
		include = d.operation.BooleanValue(value.Ref)
	case ast.ValueKindVariable:
		val, valid := d.getVariableValue(d.operation.VariableValueNameString(value.Ref))
		if !valid {
			return
		}
		include = ast.BooleanValue(val)
	default:
		return
	}
	if include {
		d.operation.RemoveDirectiveFromNode(d.Ancestors[len(d.Ancestors)-1], ref)
	} else {
		d.handleRemoveNode()
	}
}

func (d *directiveIncludeSkipVisitor) getVariableValue(name string) (value, valid bool) {
	val, err := jsonparser.GetBoolean(d.operation.Input.Variables, name)
	if err == nil {
		return val, true
	}
	for i := range d.operation.VariableDefinitions {
		definitionName := d.operation.VariableDefinitionNameString(i)
		if definitionName == name {
			if d.operation.VariableDefinitions[i].DefaultValue.IsDefined {
				return bool(d.operation.BooleanValue(d.operation.VariableDefinitions[i].DefaultValue.Value.Ref)), true
			}
		}
	}
	return false, false
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
