package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

var (
	parseValuePossibleKeywords = []token.Keyword{token.FALSE, token.TRUE, token.VARIABLE, token.INTEGER, token.FLOAT, token.STRING, token.NULL, token.IDENT, token.SQUAREBRACKETOPEN, token.SQUAREBRACKETCLOSE}
)

func (p *Parser) parseValue() (val document.Value, err error) {

	tok, err := p.read(WithReadRepeat())
	if err != nil {
		return nil, err
	}

	switch tok.Keyword {
	case token.FALSE, token.TRUE:
		val, err = p.parseBoolValue()
		return
	case token.VARIABLE:
		val, err = p.parseVariableValue()
		return
	case token.INTEGER:
		val, err = p.parseIntValue()
		return
	case token.FLOAT:
		val, err = p.parseFloatValue()
		return
	case token.STRING:
		val, err = p.parseStringValue()
		return
	case token.NULL:
		p.read()
		val = document.NullValue{}
		return
	case token.IDENT:
		val, err = p.parseEnumValue()
		return
	case token.SQUAREBRACKETOPEN:
		val, err = p.parseListValue()
		return
	case token.CURLYBRACKETOPEN:
		val, err = p.parseObjectValue()
		return
	default:
		return nil, newErrInvalidType(tok.Position, "parseValue", fmt.Sprintf("%v", parseValuePossibleKeywords), string(tok.Keyword))
	}
}
