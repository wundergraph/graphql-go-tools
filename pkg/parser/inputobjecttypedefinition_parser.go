package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInputObjectTypeDefinition(description *token.Token, index *[]int) error {

	start, err := p.readExpect(keyword.INPUT, "parseInputObjectTypeDefinition")
	if err != nil {
		return err
	}

	ident, err := p.readExpect(keyword.IDENT, "parseInputObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeInputObjectTypeDefinition()
	definition.Name = p.putByteSliceReference(ident.Literal)

	if description != nil {
		definition.Position.MergeStartIntoStart(description.TextPosition)
		definition.Description = description.Literal
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	err = p.parseInputFieldsDefinition(&definition.InputFieldsDefinition)
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putInputObjectTypeDefinition(definition))

	return nil
}
