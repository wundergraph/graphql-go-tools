package astprinter

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/input"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"io"
)

type Printer struct {
	printer printer
}

func (p *Printer) SetInput(document *ast.Document, input *input.Input) {
	p.printer.document = document
	p.printer.input = input
}

func (p *Printer) Print(out io.Writer) error {
	p.printer.err = nil
	p.printer.out = out
	astvisitor.Visit(p.printer.document, &p.printer)
	return p.printer.err
}

type printer struct {
	document *ast.Document
	input    *input.Input
	out      io.Writer
	err      error
}

func (p *printer) write(data []byte) {
	if p.err != nil {
		return
	}
	_, p.err = p.out.Write(data)
}

func (p *printer) EnterOperationDefinition(ref int, ancestors []ast.Node) {
	switch p.document.OperationDefinitions[ref].OperationType {
	case ast.OperationTypeQuery:
		p.write(literal.QUERY)
	case ast.OperationTypeMutation:
		p.write(literal.MUTATION)
	case ast.OperationTypeSubscription:
		p.write(literal.SUBSCRIPTION)
	}

	p.write(literal.SPACE)

	if p.document.OperationDefinitions[ref].Name.Length() > 0 {
		p.write(p.input.ByteSlice(p.document.OperationDefinitions[ref].Name))
		p.write(literal.SPACE)
	}
}

func (p *printer) LeaveOperationDefinition(ref int) {
	p.write(literal.SPACE)
}

func (p *printer) EnterSelectionSet(set ast.SelectionSet, ancestors []ast.Node) {
	p.write(literal.LBRACE)
}

func (p *printer) LeaveSelectionSet(set ast.SelectionSet, hasNext bool) {
	p.write(literal.RBRACE)
}

func (p *printer) EnterField(ref int, ancestors []ast.Node, hasSelections bool) {
	p.write(p.input.ByteSlice(p.document.Fields[ref].Name))
	if hasSelections {
		p.write(literal.SPACE)
	}
}

func (p *printer) LeaveField(ref int, hasNext bool) {
	if hasNext {
		p.write(literal.SPACE)
	}
}

func (p *printer) EnterFragmentSpread(ref int) {
	p.write(literal.SPREAD)
	p.write(p.input.ByteSlice(p.document.FragmentSpreads[ref].FragmentName))
}

func (p *printer) LeaveFragmentSpread(ref int, hasNext bool) {

}

func (p *printer) EnterInlineFragment(ref int) {

}

func (p *printer) LeaveInlineFragment(ref int, hasNext bool) {

}

func (p *printer) EnterFragmentDefinition(ref int) {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.input.ByteSlice(p.document.FragmentDefinitions[ref].Name))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.input.ByteSlice(p.document.Types[p.document.FragmentDefinitions[ref].TypeCondition.Type].Name))
	p.write(literal.SPACE)
}

func (p *printer) LeaveFragmentDefinition(ref int) {

}
