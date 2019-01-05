package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInterfaceTypeDefinition(index *[]int) error {

	definition := p.makeInterfaceTypeDefinition()

	interfaceName, err := p.readExpect(keyword.IDENT, "parseInterfaceTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = interfaceName.Literal

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	err = p.parseFieldsDefinition(&definition.FieldsDefinition)
	if err != nil {
		return err
	}

	*index = append(*index, p.putInterfaceTypeDefinition(definition))

	return nil
}
