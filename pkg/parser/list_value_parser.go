package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedListValue(index *int) error {

	p.l.Read()

	listValue := p.makeListValue(index)

	for {

		peeked := p.l.Peek(true)

		if peeked == keyword.SQUAREBRACKETCLOSE {
			p.l.Read()
			p.putListValue(listValue, *index)
			return nil

		} else {

			var next int
			err := p.parseValue(&next)
			if err != nil {
				return err
			}

			listValue = append(listValue, next)
		}
	}
}
