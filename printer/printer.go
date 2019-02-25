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
}

func (p *Printer) PrintExecutableSchema(out io.Writer) (err error) {

	p.out = out
	p.w.WalkExecutable()

	operations := p.w.OperationDefinitionIterable()
	for operations.Next() {
		operation := operations.Value()
		if operation.SelectionSet != -1 {
			err = p.printSelectionSet(operation.SelectionSet)
		}
	}

	return err
}

func (p *Printer) printSelectionSet(ref int) (err error) {

	_, err = p.out.Write(literal.CURLYBRACKETOPEN)
	if err != nil {
		return err
	}

	set := p.l.SelectionSetContentsIterator(ref)
	var addSpace bool
	for set.Next() {

		if addSpace {
			_, err = p.out.Write(literal.SPACE)
		}

		kind, ref := set.Value()
		switch kind {
		case lookup.FIELD:
			err = p.printField(ref)
		case lookup.FRAGMENT_SPREAD:
			err = p.printFragmentSpread(ref)
		case lookup.INLINE_FRAGMENT:
			err = p.printInlineFragment(ref)
		}

		addSpace = true
	}

	_, err = p.out.Write(literal.CURLYBRACKETCLOSE)
	if err != nil {
		return err
	}

	return
}

func (p *Printer) printField(ref int) (err error) {

	field := p.l.Field(ref)
	_, err = p.out.Write(p.p.CachedByteSlice(field.Name))

	if field.SelectionSet != -1 {
		_, err = p.out.Write(literal.SPACE)
		if err != nil {
			return err
		}
		err = p.printSelectionSet(field.SelectionSet)
		if err != nil {
			return err
		}
	}

	if field.ArgumentSet != -1 {
		err = p.printArgumentSet(field.ArgumentSet)
	}

	return
}

func (p *Printer) printFragmentSpread(ref int) (err error) {

	spread := p.l.FragmentSpread(ref)
	_, err = p.out.Write(literal.SPREAD)
	_, err = p.out.Write(p.p.CachedByteSlice(spread.FragmentName))

	return
}

func (p *Printer) printInlineFragment(ref int) (err error) {

	inline := p.l.InlineFragment(ref)
	_, err = p.out.Write(literal.SPREAD)

	if inline.TypeCondition != -1 {
		typeCondition := p.l.Type(inline.TypeCondition)
		_, err = p.out.Write(literal.ON)
		_, err = p.out.Write(literal.SPACE)
		_, err = p.out.Write(p.p.CachedByteSlice(typeCondition.Name))
		_, err = p.out.Write(literal.SPACE)
	}

	err = p.printSelectionSet(inline.SelectionSet)

	return
}

func (p *Printer) printArgumentSet(ref int) (err error) {

	_, err = p.out.Write(literal.BRACKETOPEN)
	if err != nil {
		return err
	}

	set := p.l.ArgumentSet(ref)
	iter := p.l.ArgumentsIterable(set)
	var addSpace bool
	for iter.Next() {

		if addSpace {
			_, err = p.out.Write(literal.SPACE)
			if err != nil {
				return err
			}
		}

		argument, _ := iter.Value()
		err = p.printArgument(argument)
		if err != nil {
			return err
		}
	}

	_, err = p.out.Write(literal.BRACKETCLOSE)
	if err != nil {
		return err
	}

	return
}

func (p *Printer) printArgument(arg document.Argument) (err error) {

	_, err = p.out.Write(p.p.CachedByteSlice(arg.Name))
	if err != nil {
		return err
	}

	_, err = p.out.Write(literal.COLON)
	if err != nil {
		return err
	}

	err = p.printValue(arg.Value)

	return
}

func (p *Printer) printValue(ref int) (err error) {

	value := p.l.Value(ref)

	switch value.ValueType {
	case document.ValueTypeBoolean, document.ValueTypeInt, document.ValueTypeFloat, document.ValueTypeEnum:
		_, err = p.out.Write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeNull:
		_, err = p.out.Write(literal.NULL)
	case document.ValueTypeString:
		_, err = p.out.Write(literal.QUOTE)
		_, err = p.out.Write(p.p.ByteSlice(value.Raw))
		_, err = p.out.Write(literal.QUOTE)
	case document.ValueTypeVariable:
		_, err = p.out.Write(literal.DOLLAR)
		_, err = p.out.Write(p.p.ByteSlice(value.Raw))
	case document.ValueTypeObject:
		err = p.printObjectValue(value.Reference)
	case document.ValueTypeList:
		err = p.printListValue(value.Reference)
	}

	return
}

func (p *Printer) printObjectValue(ref int) (err error) {

	_, err = p.out.Write(literal.CURLYBRACKETOPEN)
	if err != nil {
		return err
	}

	objectValue := p.l.ObjectValue(ref)
	fields := p.l.ObjectFieldsIterator(objectValue)
	var addComma bool
	for fields.Next() {
		if addComma {
			_, err = p.out.Write(literal.COMMA)
		}

		field, _ := fields.Value()
		err = p.printObjectField(field)

		addComma = true
	}

	_, err = p.out.Write(literal.CURLYBRACKETCLOSE)
	if err != nil {
		return err
	}

	return
}

func (p *Printer) printObjectField(field document.ObjectField) (err error) {
	_, err = p.out.Write(p.p.CachedByteSlice(field.Name))
	_, err = p.out.Write(literal.COLON)
	err = p.printValue(field.Value)
	return
}

func (p *Printer) printListValue(ref int) (err error) {

	_, err = p.out.Write(literal.SQUAREBRACKETOPEN)
	if err != nil {
		return err
	}

	list := p.l.ListValue(ref)
	var addComma bool
	for _, valueRef := range list {

		if addComma {
			_, err = p.out.Write(literal.COMMA)
			if err != nil {
				return err
			}
		}

		err = p.printValue(valueRef)
		addComma = true
	}

	_, err = p.out.Write(literal.SQUAREBRACKETCLOSE)
	if err != nil {
		return err
	}

	return
}
