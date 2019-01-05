package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedIntValue() (val document.Value, err error) {

	integerToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeInt
	val.IntValue, err = transform.StringToInt32(integerToken.Literal)

	return
}
