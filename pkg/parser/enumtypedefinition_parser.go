package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseEnumTypeDefinition(index *[]int) error {

	definition := p.makeEnumTypeDefinition()

	ident, err := p.readExpect(keyword.IDENT, "parseEnumTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = ident.Literal

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	err = p.parseEnumValuesDefinition(&definition.EnumValuesDefinition)
	if err != nil {
		return err
	}

	*index = append(*index, p.putEnumTypeDefinition(definition))

	return nil
}
