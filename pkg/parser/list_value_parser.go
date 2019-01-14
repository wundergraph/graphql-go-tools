package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedListValue(index *int) error {

	_, err := p.l.Read()
	if err != nil {
		return nil
	}

	listValue := p.makeListValue(index)
	var peeked keyword.Keyword

	for {
		peeked, err = p.l.Peek(true)
		if err != nil {
			return err
		}

		switch peeked {
		case keyword.SQUAREBRACKETCLOSE:
			_, err = p.l.Read()
			if err != nil {
				return err
			}

			p.putListValue(listValue, *index)
			return nil

		default:

			var next int
			err := p.parseValue(&next)
			if err != nil {
				return err
			}

			listValue = append(listValue, next)
		}
	}
}
