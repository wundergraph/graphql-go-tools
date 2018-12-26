package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseUnionTypeDefinition() (unionTypeDefinition document.UnionTypeDefinition, err error) {

	unionName, err := p.readExpect(keyword.IDENT, "parseUnionTypeDefinition")
	if err != nil {
		return
	}

	unionTypeDefinition.Name = string(unionName.Literal)

	unionTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	hasMembers, err := p.peekExpect(keyword.EQUALS, true)
	if err != nil {
		return
	}

	if !hasMembers {
		return
	}

	for {

		member, err := p.readExpect(keyword.IDENT, "parseUnionTypeDefinition")
		if err != nil {
			return unionTypeDefinition, err
		}

		unionTypeDefinition.UnionMemberTypes = append(unionTypeDefinition.UnionMemberTypes, string(member.Literal))

		hasAnother, err := p.peekExpect(keyword.PIPE, true)
		if err != nil {
			return unionTypeDefinition, err
		}

		if !hasAnother {
			return unionTypeDefinition, err
		}
	}
}
