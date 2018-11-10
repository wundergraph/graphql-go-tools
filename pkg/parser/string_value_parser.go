package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parseStringValue() (val document.StringValue, err error) {

	tok, err := p.read(WithWhitelist(token.STRING))
	if err != nil {
		return val, err
	}

	val.Val = string(transform.TrimWhitespace(tok.Literal))

	return
}
