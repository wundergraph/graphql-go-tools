package astnormalization

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

func directiveIncludeSkip(walker *astvisitor.Walker) {
	visitor := directiveIncludeSkipVisitor{}
	walker.RegisterEnterDocumentVisitor(&visitor)
	walker.RegisterEnterDirectiveVisitor(&visitor)
}

type directiveIncludeSkipVisitor struct {
	operation, definition *ast.Document
}

func (d *directiveIncludeSkipVisitor) EnterDocument(operation, definition *ast.Document) astvisitor.Instruction {
	d.operation = operation
	d.definition = definition
	return astvisitor.Instruction{}
}

func (d *directiveIncludeSkipVisitor) EnterDirective(ref int, info astvisitor.Info) astvisitor.Instruction {

	name := d.operation.DirectiveNameBytes(ref)

	switch {
	case bytes.Equal(name, literal.INCLUDE):
		d.handleInclude(ref, info)
	case bytes.Equal(name, literal.SKIP):
		d.handleSkip(ref, info)
	}

	return astvisitor.Instruction{}
}

func (d *directiveIncludeSkipVisitor) handleSkip(ref int, info astvisitor.Info) {
	if len(d.operation.Directives[ref].Arguments.Refs) != 1 {
		return
	}
	arg := d.operation.Directives[ref].Arguments.Refs[0]
	if !bytes.Equal(d.operation.ArgumentName(arg), literal.IF) {
		return
	}
	value := d.operation.ArgumentValue(arg)
	if value.Kind != ast.ValueKindBoolean {
		return
	}
	include := d.operation.BooleanValue(value.Ref)
	switch include {
	case false:
		d.operation.RemoveDirectiveFromNode(info.Ancestors[len(info.Ancestors)-1], ref)
	case true:
		if len(info.Ancestors) < 2 {
			return
		}
		d.operation.RemoveNodeFromNode(info.Ancestors[len(info.Ancestors)-1], info.Ancestors[len(info.Ancestors)-2])
	}
}

func (d *directiveIncludeSkipVisitor) handleInclude(ref int, info astvisitor.Info) {
	if len(d.operation.Directives[ref].Arguments.Refs) != 1 {
		return
	}
	arg := d.operation.Directives[ref].Arguments.Refs[0]
	if !bytes.Equal(d.operation.ArgumentName(arg), literal.IF) {
		return
	}
	value := d.operation.ArgumentValue(arg)
	if value.Kind != ast.ValueKindBoolean {
		return
	}
	include := d.operation.BooleanValue(value.Ref)
	switch include {
	case true:
		d.operation.RemoveDirectiveFromNode(info.Ancestors[len(info.Ancestors)-1], ref)
	case false:
		if len(info.Ancestors) < 2 {
			return
		}
		d.operation.RemoveNodeFromNode(info.Ancestors[len(info.Ancestors)-1], info.Ancestors[len(info.Ancestors)-2])
	}
}
