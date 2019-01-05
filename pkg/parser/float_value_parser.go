package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedFloatValue() (val document.Value, err error) {

	floatToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeFloat
	val.FloatValue, err = transform.StringToFloat32(floatToken.Literal)

	return
}
