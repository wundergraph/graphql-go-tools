package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

var (
	parseValuePossibleKeywords = []keyword.Keyword{keyword.FALSE, keyword.TRUE, keyword.VARIABLE, keyword.INTEGER, keyword.FLOAT, keyword.STRING, keyword.NULL, keyword.IDENT, keyword.SQUAREBRACKETOPEN, keyword.SQUAREBRACKETCLOSE}
)

func (p *Parser) parseValue(index *int) (err error) {

	key := p.l.Peek(true)
	value := p.makeValue(index)
	value.Position.MergeStartIntoStart(p.TextPosition())

	switch key {
	case keyword.FALSE, keyword.TRUE:
		value.ValueType = document.ValueTypeBoolean
		p.parsePeekedBoolValue(&value.Reference)
	case keyword.VARIABLE:
		value.ValueType = document.ValueTypeVariable
		p.parsePeekedByteSlice(&value.Reference)
	case keyword.INTEGER:
		value.ValueType = document.ValueTypeInt
		err = p.parsePeekedIntValue(&value.Reference)
	case keyword.FLOAT:
		value.ValueType = document.ValueTypeFloat
		err = p.parsePeekedFloatValue(&value.Reference)
	case keyword.STRING:
		value.ValueType = document.ValueTypeString
		p.parsePeekedByteSlice(&value.Reference)
	case keyword.NULL:
		value.ValueType = document.ValueTypeNull
		p.l.Read()
	case keyword.IDENT:
		value.ValueType = document.ValueTypeEnum
		p.parsePeekedByteSlice(&value.Reference)
	case keyword.SQUAREBRACKETOPEN:
		value.ValueType = document.ValueTypeList
		err = p.parsePeekedListValue(&value.Reference)
	case keyword.CURLYBRACKETOPEN:
		value.ValueType = document.ValueTypeObject
		err = p.parsePeekedObjectValue(&value.Reference)
	default:
		invalidToken := p.l.Read()
		return newErrInvalidType(invalidToken.TextPosition, "parseValue", fmt.Sprintf("%v", parseValuePossibleKeywords), string(invalidToken.Keyword))
	}

	value.Position.MergeStartIntoEnd(p.TextPosition())
	p.putValue(value, *index)

	return err
}
