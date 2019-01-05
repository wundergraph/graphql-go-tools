package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

func (p *Parser) parsePeekedEnumValue() (val document.Value, err error) {

	enumToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeEnum
	val.EnumValue = enumToken.Literal

	return
}
