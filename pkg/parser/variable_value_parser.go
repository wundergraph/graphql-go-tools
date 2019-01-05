package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
)

func (p *Parser) parsePeekedVariableValue() (val document.Value, err error) {

	variableToken, err := p.l.Read()
	if err != nil {
		return val, err
	}

	val.ValueType = document.ValueTypeVariable
	val.VariableValue = variableToken.Literal

	return
}
