package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) parseObjectTypeDefinition(hasDescription, isExtend bool, description token.Token) error {

	start, err := p.readExpect(keyword.TYPE, "parseObjectTypeDefinition")
	if err != nil {
		return err
	}

	objectTypeName, err := p.readExpect(keyword.IDENT, "parseObjectTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeObjectTypeDefinition()
	definition.Name = objectTypeName.Literal
	definition.IsExtend = isExtend

	if hasDescription {
		definition.Position.MergeStartIntoStart(description.TextPosition)
		definition.Description = description.Literal
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

	definition.FieldsDefinition, err = p.parseFieldDefinitions()
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	p.putObjectTypeDefinition(definition)

	return nil
}
