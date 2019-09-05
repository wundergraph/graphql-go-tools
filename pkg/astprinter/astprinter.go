package astprinter

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/fastastvisitor"
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
	walker     fastastvisitor.SimpleWalker
	registered bool
}

func (p *Printer) Print(document, definition *ast.Document, out io.Writer) error {
	p.visitor.err = nil
	p.visitor.document = document
	p.visitor.out = out
	p.visitor.SimpleWalker = &p.walker
	if !p.registered {
		p.walker.SetVisitor(&p.visitor)
	}
	return p.walker.Walk(p.visitor.document, definition)
}

type printVisitor struct {
	*fastastvisitor.SimpleWalker
	document *ast.Document
	out      io.Writer
	err      error
}

func (p *printVisitor) EnterDocument(operation, definition *ast.Document) {

}

func (p *printVisitor) LeaveDocument(operation, definition *ast.Document) {

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

func (p *printVisitor) EnterDirective(ref int) {
	p.write(literal.AT)
	p.write(p.document.DirectiveNameBytes(ref))
}

func (p *printVisitor) LeaveDirective(ref int) {
	ancestor := p.Ancestors[len(p.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindOperationDefinition:
		p.write(literal.SPACE)
	case ast.NodeKindField:
		if len(p.SelectionsAfter) > 0 || p.document.FieldHasSelections(ancestor.Ref) {
			p.write(literal.SPACE)
		}
	}
}

func (p *printVisitor) EnterVariableDefinition(ref int) {
	if !p.document.VariableDefinitionsBefore(ref) {
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
}

func (p *printVisitor) LeaveVariableDefinition(ref int) {
	if !p.document.VariableDefinitionsAfter(ref) {
		p.write(literal.RPAREN)
	} else {
		p.write(literal.COMMA)
		p.write(literal.SPACE)
	}
}

func (p *printVisitor) EnterArgument(ref int) {
	if len(p.document.ArgumentsBefore(p.Ancestors[len(p.Ancestors)-1], ref)) == 0 {
		p.write(literal.LPAREN)
	} else {
		p.write(literal.COMMA)
		p.write(literal.SPACE)
	}
	p.must(p.document.PrintArgument(ref, p.out))

}

func (p *printVisitor) LeaveArgument(ref int) {
	if len(p.document.ArgumentsAfter(p.Ancestors[len(p.Ancestors)-1], ref)) == 0 {
		p.write(literal.RPAREN)
	}

}

func (p *printVisitor) EnterOperationDefinition(ref int) {

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

}

func (p *printVisitor) LeaveOperationDefinition(ref int) {
	if !p.document.OperationDefinitionIsLastRootNode(ref) {
		p.write(literal.SPACE)
	}
}

func (p *printVisitor) EnterSelectionSet(ref int) {
	p.write(literal.LBRACE)
}

func (p *printVisitor) LeaveSelectionSet(ref int) {
	p.write(literal.RBRACE)
}

func (p *printVisitor) EnterField(ref int) {
	if p.document.Fields[ref].Alias.IsDefined {
		p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Alias.Name))
		p.write(literal.COLON)
		p.write(literal.SPACE)
	}
	p.write(p.document.Input.ByteSlice(p.document.Fields[ref].Name))
	if p.document.FieldHasSelections(ref) || p.document.FieldHasDirectives(ref) {
		p.write(literal.SPACE)
	}
}

func (p *printVisitor) LeaveField(ref int) {
	if !p.document.FieldHasDirectives(ref) && len(p.SelectionsAfter) != 0 {
		p.write(literal.SPACE)
	}
}

func (p *printVisitor) EnterFragmentSpread(ref int) {
	p.write(literal.SPREAD)
	p.write(p.document.Input.ByteSlice(p.document.FragmentSpreads[ref].FragmentName))

}

func (p *printVisitor) LeaveFragmentSpread(ref int) {

}

func (p *printVisitor) EnterInlineFragment(ref int) {
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

}

func (p *printVisitor) LeaveInlineFragment(ref int) {
	ancestor := p.Ancestors[len(p.Ancestors)-1]
	if p.document.SelectionsAfterInlineFragment(ref, ancestor) {
		p.write(literal.SPACE)
	}
}

func (p *printVisitor) EnterFragmentDefinition(ref int) {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.FragmentDefinitions[ref].Name))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.document.Input.ByteSlice(p.document.Types[p.document.FragmentDefinitions[ref].TypeCondition.Type].Name))
	p.write(literal.SPACE)

}

func (p *printVisitor) LeaveFragmentDefinition(ref int) {
	if !p.document.FragmentDefinitionIsLastRootNode(ref) {
		p.write(literal.SPACE)
	}
}
