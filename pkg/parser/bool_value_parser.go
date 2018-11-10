package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseBoolValue() (val document.BooleanValue, err error) {

	tok, err := p.read(WithWhitelist(token.TRUE, token.FALSE))
	if err != nil {
		return val, err
	}

	val.Val = tok.Keyword == token.TRUE

	return
}
