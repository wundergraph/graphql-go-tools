package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseType() (ref document.Type, err error) {

	next, err := p.l.Peek(true)
	if err != nil {
		return nil, err
	}

	if next == keyword.SQUAREBRACKETOPEN {
		return p.parseListType()
	}

	return p.parseNamedType()

}
