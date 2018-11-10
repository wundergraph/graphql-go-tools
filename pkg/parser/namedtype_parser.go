package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseNamedType() (namedType document.NamedType, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	namedType.Name = string(tok.Literal)

	tok, matched, err := p.readOptionalToken(token.BANG)
	if err != nil {
		return
	}

	if matched {
		namedType.NonNull = true
	}

	return
}
