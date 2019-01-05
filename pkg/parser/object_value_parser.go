package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedObjectValue() (value document.Value, err error) {

	value.ValueType = document.ValueTypeObject

	_, err = p.l.Read()
	if err != nil {
		return value, err
	}

	var peeked keyword.Keyword

	for {
		peeked, err = p.l.Peek(true)
		if err != nil {
			return value, err
		}

		switch peeked {
		case keyword.CURLYBRACKETCLOSE:
			_, err = p.l.Read()
			return value, err
		case keyword.IDENT:
			identToken, err := p.l.Read()
			if err != nil {
				return value, err
			}

			var field document.ObjectField
			field.Name = identToken.Literal

			expectColon, err := p.l.Peek(true)
			if err != nil {
				return value, err
			}

			if expectColon != keyword.COLON {
				return value, fmt.Errorf("parsePeekedObjectValue: expected colon, got %s", expectColon)
			}

			_, err = p.l.Read()
			if err != nil {
				return value, err
			}

			field.Value, err = p.parseValue()
			if err != nil {
				return value, err
			}

			value.ObjectValue = append(value.ObjectValue, field)

		default:
			return value, fmt.Errorf("parsePeekedObjectValue: expected }/ident, got: %s", peeked)
		}
	}
}
