package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedObjectValue(index *int) error {

	p.l.Read()

	objectValue := p.makeObjectValue(index)

	var peeked keyword.Keyword

	for {
		peeked = p.l.Peek(true)

		switch peeked {
		case keyword.RBRACE:

			p.l.Read()
			p.putObjectValue(objectValue, index)
			return nil

		case keyword.IDENT:

			identToken := p.l.Read()
			field := document.ObjectField{
				Name: identToken.Literal,
			}

			_, err := p.readExpect(keyword.COLON, "parsePeekedObjectValue")
			if err != nil {
				return err
			}

			field.Value, err = p.parseValue()
			if err != nil {
				return err
			}

			objectValue = append(objectValue, p.putObjectField(field))

		default:
			return fmt.Errorf("parsePeekedObjectValue: expected }/ident, got: %s", peeked)
		}
	}
}
