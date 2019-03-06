package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInterfaceTypeDefinition(description *token.Token, index *[]int) error {

	start, err := p.readExpect(keyword.INTERFACE, "parseInterfaceTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeInterfaceTypeDefinition()

	if description != nil {
		definition.Position.MergeStartIntoStart(description.TextPosition)
		definition.Description = description.Literal
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	interfaceName, err := p.readExpect(keyword.IDENT, "parseInterfaceTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = p.putByteSliceReference(interfaceName.Literal)

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	err = p.parseFieldsDefinition(&definition.FieldsDefinition)
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putInterfaceTypeDefinition(definition))

	return nil
}
