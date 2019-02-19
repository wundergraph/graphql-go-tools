package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseField() (ref int, err error) {

	var field document.Field
	p.initField(&field)

	firstIdent := p.l.Read()
	field.Name = p.putByteSliceReference(firstIdent.Literal)
	field.Position.MergeStartIntoStart(firstIdent.TextPosition)

	hasAlias := p.peekExpect(keyword.COLON, true)

	if hasAlias {
		field.Alias = field.Name
		fieldName, err := p.readExpect(keyword.IDENT, "parseField")
		if err != nil {
			return ref, err
		}

		field.Name = p.putByteSliceReference(fieldName.Literal)
	}

	err = p.parseArgumentSet(&field.ArgumentSet)
	if err != nil {
		return ref, err
	}

	err = p.parseDirectives(&field.DirectiveSet)
	if err != nil {
		return ref, err
	}

	err = p.parseSelectionSet(&field.SelectionSet)

	var arguments document.ArgumentSet
	if field.ArgumentSet != -1 {
		arguments = p.ParsedDefinitions.ArgumentSets[field.ArgumentSet]
	}

	directives := p.ParsedDefinitions.DirectiveSets[field.DirectiveSet]

	var selectionSet document.SelectionSet
	if field.SelectionSet != -1 {
		selectionSet = p.ParsedDefinitions.SelectionSets[field.SelectionSet]
	}

	if len(arguments) == 0 && len(directives) == 0 && selectionSet.IsEmpty() {
		field.Position.MergeEndIntoEnd(firstIdent.TextPosition)
	} else {
		field.Position.MergeStartIntoEnd(p.TextPosition())
	}

	return p.putField(field), err
}
