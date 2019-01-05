package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedBoolValue() (val document.Value, err error) {

	trueFalseToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeBoolean
	val.BooleanValue = trueFalseToken.Keyword == keyword.TRUE

	return
}
