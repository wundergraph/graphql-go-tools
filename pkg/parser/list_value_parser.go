package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseListValue() (val document.ListValue, err error) {

	_, err = p.read(WithWhitelist(token.SQUAREBRACKETOPEN))
	if err != nil {
		return
	}

	err = p.readAllUntil(token.SQUAREBRACKETCLOSE, WithReadRepeat()).foreach(func(tok token.Token) bool {
		listItem, err := p.parseValue()
		if err != nil {
			return false
		}

		val.Values = append(val.Values, listItem)
		return true
	})

	return
}
