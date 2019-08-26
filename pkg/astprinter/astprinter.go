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
	buff := &bytes.Buffer{}
	err := Print(document, definition, buff)
	out := buff.String()
	return out, err
}

type Printer struct {
	visitor    printVisitor
	walker     astvisitor.Walker
	registered bool
}

func (p *Printer) Print(document, definition *ast.Document, out io.Writer) error {
	p.visitor.err = nil
	p.visitor.document = document
	p.visitor.out = out
	if !p.registered {
		p.walker.RegisterAllNodesVisitor(&p.visitor)
	}
	return p.walker.Walk(p.visitor.document, definition)
}

type printVisitor struct {
	document *ast.Document
	out      io.Writer
	err      error
}

func (p *printVisitor) write(data []byte) {
	if p.err != nil {
		return
	}
	_, p.err = p.out.Write(data)
}

func (p *printVisitor) must(err error) {
	if p.err != nil {
		return
	}
	p.err = err
}

func (p *printVisitor) EnterDirective(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.AT)
	p.write(p.document.DirectiveNameBytes(ref))
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveDirective(ref int, info astvisitor.Info) astvisitor.Instruction {
	switch info.Ancestors[len(info.Ancestors)-1].Kind {
	case ast.NodeKindOperationDefinition, ast.NodeKindField:
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterVariableDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.VariableDefinitionsBefore) == 0 {
		p.write(literal.LPAREN)
	}

	p.must(p.document.PrintValue(p.document.VariableDefinitions[ref].VariableValue, p.out))
	p.write(literal.COLON)
	p.write(literal.SPACE)

	p.must(p.document.PrintType(p.document.VariableDefinitions[ref].Type, p.out))

	if p.document.VariableDefinitions[ref].DefaultValue.IsDefined {
		p.write(literal.SPACE)
		p.write(literal.EQUALS)
		p.write(literal.SPACE)
		p.must(p.document.PrintValue(p.document.VariableDefinitions[ref].DefaultValue.Value, p.out))
	}

	if p.document.VariableDefinitions[ref].HasDirectives {
		p.write(literal.SPACE)
	}

	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveVariableDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.VariableDefinitionsAfter) == 0 {
		p.write(literal.RPAREN)
	} else {
		p.write(literal.COMMA)
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterArgument(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.ArgumentsBefore) == 0 {
		p.write(literal.LPAREN)
	} else {
		p.write(literal.COMMA)
		p.write(literal.SPACE)
	}
	p.must(p.document.PrintArgument(ref, p.out))
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveArgument(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.ArgumentsAfter) == 0 {
		p.write(literal.RPAREN)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterOperationDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {

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
		if !p.document.OperationDefinitions[ref].HasVariableDefinitions {
			p.write(literal.SPACE)
		}
	}

	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveOperationDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {

	if !info.IsLastRootNode {
		p.write(literal.SPACE)
	}

	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.LBRACE)
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveSelectionSet(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.RBRACE)
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterField(ref int, info astvisitor.Info) astvisitor.Instruction {
	if p.document.Fields[ref].Alias.IsDefined {
		p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Alias.Name))
		p.write(literal.COLON)
		p.write(literal.SPACE)
	}
	p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Name))
	if info.HasSelections {
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveField(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.SelectionsAfter) != 0 {
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterFragmentSpread(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.SPREAD)
	p.write(p.document.Input.ByteSlice(p.document.FragmentSpreads[ref].FragmentName))
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveFragmentSpread(ref int, info astvisitor.Info) astvisitor.Instruction {
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterInlineFragment(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.SPREAD)
	if p.document.InlineFragments[ref].TypeCondition.Type != -1 {
		p.write(literal.SPACE)
		p.write(literal.ON)
		p.write(literal.SPACE)
		p.write(p.document.Input.ByteSlice(p.document.Types[p.document.InlineFragments[ref].TypeCondition.Type].Name))
		p.write(literal.SPACE)
	} else if p.document.InlineFragments[ref].HasDirectives {
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveInlineFragment(ref int, info astvisitor.Info) astvisitor.Instruction {
	if len(info.SelectionsAfter) > 0 {
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}

func (p *printVisitor) EnterFragmentDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.FragmentDefinitions[ref].Name))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.Types[p.document.FragmentDefinitions[ref].TypeCondition.Type].Name))
	p.write(literal.SPACE)
	return astvisitor.Instruction{}
}

func (p *printVisitor) LeaveFragmentDefinition(ref int, info astvisitor.Info) astvisitor.Instruction {
	if !info.IsLastRootNode {
		p.write(literal.SPACE)
	}
	return astvisitor.Instruction{}
}
