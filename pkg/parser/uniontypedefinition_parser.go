package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseUnionTypeDefinition() (unionTypeDefinition document.UnionTypeDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	unionTypeDefinition.Name = string(tok.Literal)

	unionTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	_, matched, err := p.readOptionalToken(token.EQUALS)
	if err != nil {
		return
	}
	if !matched {
		return
	}

	unionTypeDefinition.UnionMemberTypes, err = p.parseKeywordDividedIdentifiers(token.PIPE)

	return
}
