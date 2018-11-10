package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseType() (ref document.Type, err error) {

	tok, err := p.read(WithReadRepeat())
	if err != nil {
		return
	}

	if tok.Keyword == token.SQUAREBRACKETOPEN {
		return p.parseListType()
	}

	return p.parseNamedType()

}
