package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseListType() (listType document.ListType, err error) {

	_, err = p.readExpect(keyword.SQUAREBRACKETOPEN, "parseListType")
	if err != nil {
		return listType, err
	}

	listType.Type, err = p.parseType()
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.SQUAREBRACKETCLOSE, "parseListType")
	if err != nil {
		return
	}

	listType.NonNull, err = p.peekExpect(keyword.BANG, true)
	return
}
