package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseListType() (listType document.ListType, err error) {

	_, err = p.read(WithWhitelist(token.SQUAREBRACKETOPEN))
	if err != nil {
		return
	}

	listType.Type, err = p.parseType()
	if err != nil {
		return
	}

	_, err = p.read(WithWhitelist(token.SQUAREBRACKETCLOSE))
	if err != nil {
		return
	}

	_, matched, err := p.readOptionalToken(token.BANG)

	if matched {
		listType.NonNull = true
	}

	return
}
