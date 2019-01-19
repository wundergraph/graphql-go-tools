package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDefaultValue(index *int) error {

	if hasDefaultValue := p.peekExpect(keyword.EQUALS, true); !hasDefaultValue {
		return nil
	}

	return p.parseValue(index)
}
