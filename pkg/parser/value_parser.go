package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

var (
	parseValuePossibleKeywords = []keyword.Keyword{keyword.FALSE, keyword.TRUE, keyword.VARIABLE, keyword.INTEGER, keyword.FLOAT, keyword.STRING, keyword.NULL, keyword.IDENT, keyword.SQUAREBRACKETOPEN, keyword.SQUAREBRACKETCLOSE}
)

func (p *Parser) parseValue() (val document.Value, err error) {

	key, err := p.l.Peek(true)

	switch key {
	case keyword.FALSE, keyword.TRUE:
		val, err = p.parsePeekedBoolValue()
		return
	case keyword.VARIABLE:
		val, err = p.parsePeekedVariableValue()
		return
	case keyword.INTEGER:
		val, err = p.parsePeekedIntValue()
		return
	case keyword.FLOAT:
		val, err = p.parsePeekedFloatValue()
		return
	case keyword.STRING:
		val, err = p.parsePeekedStringValue()
		return
	case keyword.NULL:
		_, err = p.l.Read()
		val = document.NullValue{}
		return
	case keyword.IDENT:
		val, err = p.parsePeekedEnumValue()
		return
	case keyword.SQUAREBRACKETOPEN:
		val, err = p.parsePeekedListValue()
		return
	case keyword.CURLYBRACKETOPEN:
		val, err = p.parsePeekedObjectValue()
		return
	default:
		invalidToken, _ := p.l.Read()
		return nil, newErrInvalidType(invalidToken.Position, "parseValue", fmt.Sprintf("%v", parseValuePossibleKeywords), string(invalidToken.Keyword))
	}
}
