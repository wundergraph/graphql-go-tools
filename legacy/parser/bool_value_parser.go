package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedBoolValue(value *document.Value) {

	tok := p.l.Read()

	value.Raw = tok.Literal

	if tok.Keyword == keyword.FALSE {
		value.Reference = 0
	} else {
		value.Reference = 1
	}
}
