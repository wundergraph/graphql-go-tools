package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseField() (field document.Field, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	firstIdent := string(tok.Literal)

	_, wasAlias, err := p.readOptionalToken(token.COLON)
	if err != nil {
		return
	}

	if wasAlias {
		field.Alias = firstIdent
		tok, err := p.read(WithWhitelist(token.IDENT))
		if err != nil {
			return field, err
		}
		field.Name = string(tok.Literal)
	} else {
		field.Name = firstIdent
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
