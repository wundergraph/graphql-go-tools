package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

var (
	parseValuePossibleKeywords = []keyword.Keyword{keyword.FALSE, keyword.TRUE, keyword.VARIABLE, keyword.INTEGER, keyword.FLOAT, keyword.STRING, keyword.NULL, keyword.IDENT, keyword.SQUAREBRACKETOPEN, keyword.SQUAREBRACKETCLOSE}
)

func (p *Parser) parseValue(index *int) error {

	key, err := p.l.Peek(true)
	if err != nil {
		return err
	}

	value := p.makeValue(index)

	switch key {
	case keyword.FALSE, keyword.TRUE:
		value.ValueType = document.ValueTypeBoolean
		err = p.parsePeekedBoolValue(&value.Reference)
	case keyword.VARIABLE:
		value.ValueType = document.ValueTypeVariable
		err = p.parsePeekedByteSlice(&value.Reference)
	case keyword.INTEGER:
		value.ValueType = document.ValueTypeInt
		err = p.parsePeekedIntValue(&value.Reference)
	case keyword.FLOAT:
		value.ValueType = document.ValueTypeFloat
		err = p.parsePeekedFloatValue(&value.Reference)
	case keyword.STRING:
		value.ValueType = document.ValueTypeString
		err = p.parsePeekedByteSlice(&value.Reference)
	case keyword.NULL:
		value.ValueType = document.ValueTypeNull
		_, err = p.l.Read()
	case keyword.IDENT:
		value.ValueType = document.ValueTypeEnum
		err = p.parsePeekedByteSlice(&value.Reference)
	case keyword.SQUAREBRACKETOPEN:
		value.ValueType = document.ValueTypeList
		err = p.parsePeekedListValue(&value.Reference)
	case keyword.CURLYBRACKETOPEN:
		value.ValueType = document.ValueTypeObject
		err = p.parsePeekedObjectValue(&value.Reference)
	default:
		invalidToken, _ := p.l.Read()
		return newErrInvalidType(invalidToken.Position, "parseValue", fmt.Sprintf("%v", parseValuePossibleKeywords), string(invalidToken.Keyword))
	}

	p.putValue(value, *index)

	return nil
}
