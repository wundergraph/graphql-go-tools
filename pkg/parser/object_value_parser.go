package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedObjectValue(index *int) error {

	_, err := p.l.Read()
	if err != nil {
		return err
	}

	objectValue := p.makeObjectValue(index)

	var peeked keyword.Keyword

	for {
		peeked, err = p.l.Peek(true)
		if err != nil {
			return err
		}

		switch peeked {
		case keyword.CURLYBRACKETCLOSE:

			_, err = p.l.Read()
			if err != nil {
				return err
			}

			p.putObjectValue(objectValue, *index)
			return nil

		case keyword.IDENT:

			identToken, err := p.l.Read()
			if err != nil {
				return err
			}

			field := document.ObjectField{
				Name: identToken.Literal,
			}

			_, err = p.readExpect(keyword.COLON, "parsePeekedObjectValue")
			if err != nil {
				return err
			}

			err = p.parseValue(&field.Value)
			if err != nil {
				return err
			}

			objectValue = append(objectValue, p.putObjectField(field))

		default:
			return fmt.Errorf("parsePeekedObjectValue: expected }/ident, got: %s", peeked)
		}
	}
}
