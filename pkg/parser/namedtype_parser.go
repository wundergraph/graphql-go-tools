package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseNamedType() (namedType document.NamedType, err error) {

	ident, err := p.readExpect(keyword.IDENT, "parseNamedType")
	if err != nil {
		return namedType, err
	}

	namedType.Name = ident.Literal
	namedType.NonNull, err = p.peekExpect(keyword.BANG, true)
	return
}
