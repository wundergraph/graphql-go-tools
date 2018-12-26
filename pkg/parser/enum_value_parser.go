package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

func (p *Parser) parsePeekedEnumValue() (val document.EnumValue, err error) {

	enumToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.Name = string(enumToken.Literal)

	return
}
