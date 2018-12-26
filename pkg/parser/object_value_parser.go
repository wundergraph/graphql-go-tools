package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedObjectValue() (objectValue document.ObjectValue, err error) {

	_, err = p.l.Read()
	if err != nil {
		return objectValue, err
	}

	var peeked keyword.Keyword

	for {
		peeked, err = p.l.Peek(true)
		if err != nil {
			return objectValue, err
		}

		switch peeked {
		case keyword.CURLYBRACKETCLOSE:
			_, err = p.l.Read()
			return objectValue, err
		case keyword.IDENT:
			identToken, err := p.l.Read()
			if err != nil {
				return objectValue, err
			}

			var field document.ObjectField
			field.Name = string(identToken.Literal)

			expectColon, err := p.l.Peek(true)
			if err != nil {
				return objectValue, err
			}

			if expectColon != keyword.COLON {
				return objectValue, fmt.Errorf("parsePeekedObjectValue: expected colon, got %s", expectColon)
			}

			_, err = p.l.Read()
			if err != nil {
				return objectValue, err
			}

			field.Value, err = p.parseValue()
			if err != nil {
				return objectValue, err
			}

			objectValue.Val = append(objectValue.Val, field)

		default:
			return objectValue, fmt.Errorf("parsePeekedObjectValue: expected }/ident, got: %s", peeked)
		}
	}

	/*	err = p.readAllUntil(keyword.CURLYBRACKETCLOSE,
		WithWhitelist(keyword.IDENT),
		WithReadRepeat()).
		foreach(func(tok token.Token) bool {

			var field document.ObjectField
			field, err = p.parseObjectField()
			if err != nil {
				return false
			}

			objectValue.Val = append(objectValue.Val, field)

			return true
		})

	return*/
}
