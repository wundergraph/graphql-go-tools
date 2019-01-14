package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDefaultValue(index *int) error {

	hasDefaultValue, err := p.peekExpect(keyword.EQUALS, true)
	if err != nil {
		return err
	}

	if !hasDefaultValue {
		return nil
	}

	return p.parseValue(index)
}
