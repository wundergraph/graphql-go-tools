package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) parseInterfaceTypeDefinition(hasDescription, isExtend bool, description token.Token) error {

	start, err := p.readExpect(keyword.INTERFACE, "parseInterfaceTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeInterfaceTypeDefinition()

	if hasDescription {
		definition.Position.MergeStartIntoStart(description.TextPosition)
		definition.Description = description.Literal
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	interfaceName, err := p.readExpect(keyword.IDENT, "parseInterfaceTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = interfaceName.Literal
	definition.IsExtend = isExtend

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	definition.FieldsDefinition, err = p.parseFieldDefinitions()
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	p.putInterfaceTypeDefinition(definition)

	return nil
}
