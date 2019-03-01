package printer

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lookup"
	"github.com/jensneuse/graphql-go-tools/pkg/parser"
	"io"
)

type Printer struct {
	l   *lookup.Lookup
	w   *lookup.Walker
	p   *parser.Parser
	out io.Writer
	err error
}

func New() *Printer {
	return &Printer{
		w: lookup.NewWalker(1024, 8),
	}
}

func (p *Printer) SetInput(parser *parser.Parser) {
	p.p = parser
	if p.l == nil {
		p.l = lookup.New(parser, 256)
	} else {
		p.l.SetParser(parser)
	}

	p.w.SetLookup(p.l)
	p.err = nil
}

func (p *Printer) write(bytes []byte) {
	if p.err != nil {
		return
	}
	_, p.err = p.out.Write(bytes)
}

func (p *Printer) PrintExecutableSchema(out io.Writer) {

	p.out = out
	p.w.WalkExecutable()

	operations := p.w.OperationDefinitionIterable()
	for operations.Next() {
		operation := operations.Value()
		p.printOperation(operation)
	}

	if p.l.HasOperationDefinitions() && p.l.HasFragmentDefinitions() {
		p.write(literal.LINETERMINATOR)
	}

	fragments := p.w.FragmentDefinitionIterable()
	var addNewLine bool
	for fragments.Next() {
		if addNewLine {
			p.write(literal.LINETERMINATOR)
		}
		fragment := fragments.Value()
		p.printFragmentDefinition(fragment)
		addNewLine = true
	}
}

func (p *Printer) printFragmentDefinition(fragment document.FragmentDefinition) {
	p.write(literal.FRAGMENT)
	p.write(literal.SPACE)
	p.write(p.p.CachedByteSlice(fragment.FragmentName))
	p.write(literal.SPACE)
	p.write(literal.ON)
	p.write(literal.SPACE)
	p.write(p.p.CachedByteSlice(p.l.Type(fragment.TypeCondition).Name))
	p.write(literal.SPACE)
	p.printSelectionSet(fragment.SelectionSet)
}

func (p *Printer) printOperation(operation document.OperationDefinition) {
	hasName := operation.Name != -1
	p.printOperationType(operation.OperationType, hasName)
	if hasName {
		p.write(p.p.CachedByteSlice(operation.Name))
		p.write(literal.SPACE)
	}
	if operation.DirectiveSet != -1 {
		p.printDirectiveSet(operation.DirectiveSet)
	}
	if operation.SelectionSet != -1 {
		p.printSelectionSet(operation.SelectionSet)
	}
}

func (p *Printer) printDirectiveSet(setRef int) {
	set := p.l.DirectiveSet(setRef)
	iter := p.l.DirectiveIterable(set)
	var addSpace bool
	for iter.Next() {
		if addSpace {
			p.write(literal.SPACE)
		}
		directive, _ := iter.Value()
		p.printDirective(directive)
		addSpace = true
	}

	if addSpace {
		p.write(literal.SPACE)
	}
}

func (p *Printer) printDirective(directive document.Directive) {
	p.write(literal.AT)
	p.write(p.p.CachedByteSlice(directive.Name))
	if directive.ArgumentSet != -1 {
		p.printArgumentSet(directive.ArgumentSet)
	}
}

func (p *Printer) printOperationType(operationType document.OperationType, hasName bool) {
	switch operationType {
	case document.OperationTypeQuery:
		if hasName {
			p.write(literal.QUERY)
			p.write(literal.SPACE)
		}
	case document.OperationTypeMutation:
		p.write(literal.MUTATION)
		p.write(literal.SPACE)
	case document.OperationTypeSubscription:
		p.write(literal.SUBSCRIPTION)
		p.write(literal.SPACE)
	}
	return
}

func (p *Printer) printSelectionSet(ref int) {

	p.write(literal.CURLYBRACKETOPEN)

	set := p.l.SelectionSetContentsIterator(ref)
	var addSpace bool
	for set.Next() {

		if addSpace {
			p.write(literal.SPACE)
		}

		kind, ref := set.Value()
		switch kind {
		case lookup.FIELD:
			p.printField(ref)
		case lookup.FRAGMENT_SPREAD:
			p.printFragmentSpread(ref)
		case lookup.INLINE_FRAGMENT:
			p.printInlineFragment(ref)
		}

		addSpace = true
	}

	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) printField(ref int) {

	field := p.l.Field(ref)
	p.write(p.p.CachedByteSlice(field.Name))

	if field.SelectionSet != -1 {
		p.write(literal.SPACE)
		p.printSelectionSet(field.SelectionSet)
	}

	if field.ArgumentSet != -1 {
		p.printArgumentSet(field.ArgumentSet)
	}

	return
}

func (p *Printer) printFragmentSpread(ref int) {
	spread := p.l.FragmentSpread(ref)
	p.write(literal.SPREAD)
	p.write(p.p.CachedByteSlice(spread.FragmentName))
}

func (p *Printer) printInlineFragment(ref int) {

	inline := p.l.InlineFragment(ref)
	p.write(literal.SPREAD)

	if inline.TypeCondition != -1 {
		typeCondition := p.l.Type(inline.TypeCondition)
		p.write(literal.ON)
		p.write(literal.SPACE)
		p.write(p.p.CachedByteSlice(typeCondition.Name))
		p.write(literal.SPACE)
	}

	p.printSelectionSet(inline.SelectionSet)
}

func (p *Printer) printArgumentSet(ref int) {

	p.write(literal.BRACKETOPEN)

	set := p.l.ArgumentSet(ref)
	iter := p.l.ArgumentsIterable(set)
	var addSpace bool
	for iter.Next() {

		if addSpace {
			p.write(literal.SPACE)
		}

		argument, _ := iter.Value()
		p.printArgument(argument)
	}

	p.write(literal.BRACKETCLOSE)
}

func (p *Printer) printArgument(arg document.Argument) {
	p.write(p.p.CachedByteSlice(arg.Name))
	p.write(literal.COLON)
	p.printValue(arg.Value)
}

func (p *Printer) printValue(ref int) {

	value := p.l.Value(ref)

	switch value.ValueType {
	case document.ValueTypeBoolean, document.ValueTypeInt, document.ValueTypeFloat, document.ValueTypeEnum:
		p.write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeNull:
		p.write(literal.NULL)
	case document.ValueTypeString:
		p.write(literal.QUOTE)
		p.write(p.p.ByteSlice(value.Raw))
		p.write(literal.QUOTE)
	case document.ValueTypeVariable:
		p.write(literal.DOLLAR)
		p.write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeObject:
		p.printObjectValue(value.Reference)
	case document.ValueTypeList:
		p.printListValue(value.Reference)
	}
}

func (p *Printer) printObjectValue(ref int) {

	p.write(literal.CURLYBRACKETOPEN)

	objectValue := p.l.ObjectValue(ref)
	fields := p.l.ObjectFieldsIterator(objectValue)
	var addComma bool
	for fields.Next() {
		if addComma {
			p.write(literal.COMMA)
		}

		field, _ := fields.Value()
		p.printObjectField(field)

		addComma = true
	}

	p.write(literal.CURLYBRACKETCLOSE)
}

func (p *Printer) printObjectField(field document.ObjectField) {
	p.write(p.p.CachedByteSlice(field.Name))
	p.write(literal.COLON)
	p.printValue(field.Value)
}

func (p *Printer) printListValue(ref int) {

	p.write(literal.SQUAREBRACKETOPEN)

	list := p.l.ListValue(ref)
	var addComma bool
	for _, valueRef := range list {

		if addComma {
			p.write(literal.COMMA)
		}

		p.printValue(valueRef)
		addComma = true
	}

	p.write(literal.SQUAREBRACKETCLOSE)
}
