package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parseIntValue() (val document.IntValue, err error) {

	tok, err := p.read(WithWhitelist(token.INTEGER))
	if err != nil {
		return val, err
	}

	val.Val, err = transform.StringSliceToInt32(tok.Literal)

	return
}
