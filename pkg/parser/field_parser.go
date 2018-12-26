package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseField() (field document.Field, err error) {

	firstIdent, err := p.l.Read()
	if err != nil {
		return field, err
	}

	field.Name = string(firstIdent.Literal)

	hasAlias, err := p.peekExpect(keyword.COLON, true)
	if err != nil {
		return field, err
	}

	if hasAlias {
		field.Alias = field.Name
		fieldName, err := p.readExpect(keyword.IDENT, "parseField")
		if err != nil {
			return field, err
		}

		field.Name = string(fieldName.Literal)
	}

	field.Arguments, err = p.parseArguments()
	if err != nil {
		return
	}

	field.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	field.SelectionSet, err = p.parseSelectionSet()

	return
}
