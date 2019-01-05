package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedListValue() (val document.Value, err error) {

	val.ValueType = document.ValueTypeList

	_, err = p.l.Read()
	if err != nil {
		return val, nil
	}

	var peeked keyword.Keyword

	for {
		peeked, err = p.l.Peek(true)
		if err != nil {
			return val, err
		}

		switch peeked {
		case keyword.SQUAREBRACKETCLOSE:
			_, err = p.l.Read()
			return val, err
		default:
			listValue, err := p.parseValue()
			if err != nil {
				return val, err
			}

			val.ListValue = append(val.ListValue, listValue)
		}
	}
}
