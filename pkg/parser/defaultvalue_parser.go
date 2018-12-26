package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDefaultValue() (val document.DefaultValue, err error) {

	hasDefaultValue, err := p.peekExpect(keyword.EQUALS, true)
	if err != nil {
		return val, err
	}

	if !hasDefaultValue {
		return
	}

	return p.parseValue()
}
