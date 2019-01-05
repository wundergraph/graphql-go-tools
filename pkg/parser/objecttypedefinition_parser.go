package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseObjectTypeDefinition(index *[]int) error {

	objectTypeName, err := p.readExpect(keyword.IDENT, "parseObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeObjectTypeDefinition()

	definition.Name = objectTypeName.Literal

	definition.ImplementsInterfaces, err = p.parseImplementsInterfaces()
	if err != nil {
		return err
	}

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	err = p.parseFieldsDefinition(&definition.FieldsDefinition)
	if err != nil {
		return err
	}

	*index = append(*index, p.putObjectTypeDefinition(definition))

	return nil
}
