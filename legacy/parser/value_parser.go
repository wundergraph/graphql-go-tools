package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

var (
	parseValuePossibleKeywords = []keyword.Keyword{keyword.FALSE, keyword.TRUE, keyword.VARIABLE, keyword.INTEGER, keyword.FLOAT, keyword.STRING, keyword.NULL, keyword.IDENT, keyword.LBRACK, keyword.RBRACK}
)

func (p *Parser) parseValue() (ref int, err error) {

	key := p.l.Peek(true)
	var value document.Value
	value, ref = p.makeValue()
	value.Position.MergeStartIntoStart(p.TextPosition())

	switch key {
	case keyword.FALSE, keyword.TRUE:
		value.ValueType = document.ValueTypeBoolean
		p.parsePeekedBoolValue(&value)
	case keyword.VARIABLE:
		value.ValueType = document.ValueTypeVariable
		p.parsePeekedByteSlice(&value)
	case keyword.INTEGER:
		value.ValueType = document.ValueTypeInt
		err = p.parsePeekedIntValue(&value)
	case keyword.FLOAT:
		value.ValueType = document.ValueTypeFloat
		err = p.parsePeekedFloatValue(&value)
	case keyword.STRING:
		value.ValueType = document.ValueTypeString
		p.parsePeekedByteSlice(&value)
	case keyword.NULL:
		value.ValueType = document.ValueTypeNull
		p.l.Read()
	case keyword.IDENT:
		value.ValueType = document.ValueTypeEnum
		p.parsePeekedByteSlice(&value)
	case keyword.LBRACK:
		value.ValueType = document.ValueTypeList
		value.Reference, err = p.parsePeekedListValue()
	case keyword.LBRACE:
		value.ValueType = document.ValueTypeObject
		err = p.parsePeekedObjectValue(&value.Reference)
	default:
		invalidToken := p.l.Read()
		err = newErrInvalidType(invalidToken.TextPosition, "parseValue", fmt.Sprintf("%v", parseValuePossibleKeywords), string(invalidToken.Keyword))
		return
	}

	value.Position.MergeStartIntoEnd(p.TextPosition())
	p.putValue(value, ref)

	return
}
