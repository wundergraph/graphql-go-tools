package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedStringValue() (val document.Value, err error) {

	stringToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeString
	val.StringValue = transform.TrimWhitespace(stringToken.Literal)

	return
}
