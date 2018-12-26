package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedFloatValue() (val document.FloatValue, err error) {

	floatToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.Val, err = transform.StringSliceToFloat32(floatToken.Literal)

	return
}
