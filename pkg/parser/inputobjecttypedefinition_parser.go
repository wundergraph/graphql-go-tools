package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputObjectTypeDefinition(index *[]int) error {

	ident, err := p.readExpect(keyword.IDENT, "parseInputObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeInputObjectTypeDefinition()

	definition.Name = ident.Literal

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	err = p.parseInputFieldsDefinition(&definition.InputValueDefinitions)
	if err != nil {
		return err
	}

	*index = append(*index, p.putInputObjectTypeDefinition(definition))

	return nil
}
