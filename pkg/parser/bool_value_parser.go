package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedBoolValue(index *int) {

	tok := p.l.Read()

	if tok.Keyword == keyword.FALSE {
		*index = 0
	} else {
		*index = 1
	}
}
