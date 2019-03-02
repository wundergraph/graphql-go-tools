package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDefaultValue() (ref int, err error) {

	if hasDefaultValue := p.peekExpect(keyword.EQUALS, true); !hasDefaultValue {
		ref = -1
		return
	}

	return p.parseValue()
}
