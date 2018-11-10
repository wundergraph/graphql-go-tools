package parser

import (
	document "github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parseFloatValue() (val document.FloatValue, err error) {

	tok, err := p.read(WithWhitelist(token.FLOAT))
	if err != nil {
		return val, err
	}

	val.Val, err = transform.StringSliceToFloat32(tok.Literal)

	return
}
