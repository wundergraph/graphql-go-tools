package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) parseInputObjectTypeDefinition(hasDescription, isExtend bool, description token.Token) error {

	start, err := p.readExpect(keyword.INPUT, "parseInputObjectTypeDefinition")
	if err != nil {
		return err
	}

	ident, err := p.readExpect(keyword.IDENT, "parseInputObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeInputObjectTypeDefinition()
	definition.Name = ident.Literal
	definition.IsExtend = isExtend

	if hasDescription {
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
	p.putInputObjectTypeDefinition(definition)

	return nil
}
