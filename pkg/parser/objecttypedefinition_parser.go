package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseObjectTypeDefinition(description *token.Token, index *[]int) error {

	start, err := p.readExpect(keyword.TYPE, "parseObjectTypeDefinition")
	if err != nil {
		return err
	}

	objectTypeName, err := p.readExpect(keyword.IDENT, "parseObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeObjectTypeDefinition()
	definition.Name = p.putByteSliceReference(objectTypeName.Literal)

	if description != nil {
		definition.Position.MergeStartIntoStart(description.TextPosition)
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	definition.ImplementsInterfaces, err = p.parseImplementsInterfaces()
	if err != nil {
		return err
	}

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	err = p.parseFieldsDefinition(&definition.FieldsDefinition)
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putObjectTypeDefinition(definition))

	return nil
}
