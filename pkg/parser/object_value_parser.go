package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseObjectValue() (objectValue document.ObjectValue, err error) {

	_, err = p.read(WithWhitelist(token.CURLYBRACKETOPEN))
	if err != nil {
		return
	}

	err = p.readAllUntil(token.CURLYBRACKETCLOSE,
		WithWhitelist(token.IDENT),
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

	return
}
