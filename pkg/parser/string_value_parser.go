package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedStringValue() (val document.StringValue, err error) {

	stringToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.Val = string(transform.TrimWhitespace(stringToken.Literal))

	return
}
