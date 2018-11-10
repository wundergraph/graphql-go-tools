package parser

import (
	// "fmt"
	document "github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseObjectField() (field document.ObjectField, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return field, err
	}

	field.Name = string(tok.Literal)

	_, err = p.read(WithWhitelist(token.COLON))
	if err != nil {
		return field, err
	}

	field.Value, err = p.parseValue()

	return
}
