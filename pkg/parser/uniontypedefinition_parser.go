package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseUnionTypeDefinition(index *[]int) error {

	unionName, err := p.readExpect(keyword.IDENT, "parseUnionTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeUnionTypeDefinition()

	definition.Name = unionName.Literal

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	shouldParseMembers := p.peekExpect(keyword.EQUALS, true)

	for shouldParseMembers {

		member, err := p.readExpect(keyword.IDENT, "parseUnionTypeDefinition")
		if err != nil {
			return err
		}

		definition.UnionMemberTypes = append(definition.UnionMemberTypes, member.Literal)

		shouldParseMembers = p.peekExpect(keyword.PIPE, true)
	}

	*index = append(*index, p.putUnionTypeDefinition(definition))
	return nil
}
