package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseEnumTypeDefinition(description *token.Token, index *[]int) error {

	start, err := p.readExpect(keyword.ENUM, "parseEnumTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeEnumTypeDefinition()

	if description != nil {
		definition.Position.MergeStartIntoStart(description.TextPosition)
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	ident, err := p.readExpect(keyword.IDENT, "parseEnumTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = p.putByteSliceReference(ident.Literal)

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	err = p.parseEnumValuesDefinition(&definition.EnumValuesDefinition)
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putEnumTypeDefinition(definition))

	return nil
}
