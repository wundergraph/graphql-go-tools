package astprinter

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"io"
)

func Print(document, definition *ast.Document, out io.Writer) error {
	printer := Printer{}
	return printer.Print(document, definition, out)
}

func PrintString(document, definition *ast.Document) (string, error) {
	printer := Printer{}
	buff := &bytes.Buffer{}
	err := printer.Print(document, definition, buff)
	out := buff.String()
	return out, err
}

type Printer struct {
	printer printer
	walker  astvisitor.Walker
}

func (p *Printer) Print(document, definition *ast.Document, out io.Writer) error {
	p.printer.err = nil
	p.printer.document = document
	p.printer.out = out
	return p.walker.Visit(p.printer.document, definition, &p.printer)
}

type printer struct {
	document *ast.Document
	out      io.Writer
	err      error
}

func (p *printer) EnterArgument(ref int, definition int, info astvisitor.Info) {

}

func (p *printer) LeaveArgument(ref int, definition int, info astvisitor.Info) {

}

func (p *printer) write(data []byte) {
	if p.err != nil {
		return
	}
	_, p.err = p.out.Write(data)
}

func (p *printer) EnterOperationDefinition(ref int, info astvisitor.Info) {

	hasName := p.document.OperationDefinitions[ref].Name.Length() > 0

	switch p.document.OperationDefinitions[ref].OperationType {
	case ast.OperationTypeQuery:
		if hasName {
			p.write(literal.QUERY)
		}
	case ast.OperationTypeMutation:
		p.write(literal.MUTATION)
	case ast.OperationTypeSubscription:
		p.write(literal.SUBSCRIPTION)
	}

	if hasName {
		p.write(literal.SPACE)
	}

	if hasName {
		p.write(p.document.Input.ByteSlice(p.document.OperationDefinitions[ref].Name))
		p.write(literal.SPACE)
	}
}

func (p *printer) LeaveOperationDefinition(ref int, info astvisitor.Info) {
	hasName := p.document.OperationDefinitions[ref].Name.Length() > 0
	if hasName {
		p.write(literal.SPACE)
	}
}

func (p *printer) EnterSelectionSet(ref int, info astvisitor.Info) (instruction astvisitor.Instruction) {
	p.write(literal.LBRACE)
	return
}

func (p *printer) LeaveSelectionSet(ref int, info astvisitor.Info) {
	p.write(literal.RBRACE)
}

func (p *printer) EnterField(ref int, info astvisitor.Info) {
	if p.document.Fields[ref].Alias.IsDefined {
		p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Alias.Name))
		p.write(literal.COLON)
		p.write(literal.SPACE)
	}
	p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Name))
	if info.HasSelections {
		p.write(literal.SPACE)
	}
}

func (p *printer) LeaveField(ref int, info astvisitor.Info) {
	if len(info.SelectionsAfter) != 0 {
		p.write(literal.SPACE)
	}
}

func (p *printer) EnterFragmentSpread(ref int, info astvisitor.Info) {
	p.write(literal.SPREAD)
	p.write(p.document.Input.ByteSlice(p.document.FragmentSpreads[ref].FragmentName))
}

func (p *printer) LeaveFragmentSpread(ref int, info astvisitor.Info) {

}

func (p *printer) EnterInlineFragment(ref int, info astvisitor.Info) {
	p.write(literal.SPREAD)
	if p.document.InlineFragments[ref].TypeCondition.Type != -1 {
		p.write(literal.SPACE)
		p.write(literal.ON)
		p.write(literal.SPACE)
		p.write(p.document.Input.ByteSlice(p.document.Types[p.document.InlineFragments[ref].TypeCondition.Type].Name))
		p.write(literal.SPACE)
	}
}

func (p *printer) LeaveInlineFragment(ref int, info astvisitor.Info) {
	if len(info.SelectionsAfter) > 0 {
		p.write(literal.SPACE)
	}
}

func (p *printer) EnterFragmentDefinition(ref int, info astvisitor.Info) {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.FragmentDefinitions[ref].Name))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.Types[p.document.FragmentDefinitions[ref].TypeCondition.Type].Name))
	p.write(literal.SPACE)
}

func (p *printer) LeaveFragmentDefinition(ref int, info astvisitor.Info) {
	if !info.IsLastRootNode {
		p.write(literal.SPACE)
	}
}
