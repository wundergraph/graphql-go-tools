package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

func (p *Parser) parsePeekedVariableValue() (val document.VariableValue, err error) {

	variableToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.Name = string(variableToken.Literal)

	return
}
