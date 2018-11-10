package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseDefaultValue() (val document.DefaultValue, err error) {

	if _, matched, err := p.readOptionalToken(token.EQUALS); err != nil || !matched {
		return val, err
	}

	val, err = p.parseValue()

	return
}
