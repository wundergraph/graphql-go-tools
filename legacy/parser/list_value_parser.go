package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parsePeekedListValue() (ref int, err error) {

	p.l.Read()

	listValue := p.IndexPoolGet()

	for {

		peeked := p.l.Peek(true)

		if peeked == keyword.RBRACK {
			p.l.Read()
			return p.putListValue(listValue), nil

		} else {

			var next int
			next, err := p.parseValue()
			if err != nil {
				return -1, err
			}

			listValue = append(listValue, next)
		}
	}
}
