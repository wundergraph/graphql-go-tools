package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedBoolValue(index *int) error {

	tok, err := p.l.Read()
	if err != nil {
		return err
	}

	if tok.Keyword == keyword.FALSE {
		*index = 0
	} else {
		*index = 1
	}

	return nil
}
