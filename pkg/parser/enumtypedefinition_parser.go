package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseEnumTypeDefinition(hasDescription, isExtend bool, description token.Token) error {

	start, err := p.readExpect(keyword.ENUM, "parseEnumTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeEnumTypeDefinition()

	if hasDescription {
		definition.Position.MergeStartIntoStart(description.TextPosition)
		definition.Description = description.Literal
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	ident, err := p.readExpect(keyword.IDENT, "parseEnumTypeDefinition")
	if err != nil {
		return err
	}

	definition.Name = ident.Literal
	definition.IsExtend = isExtend

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	definition.EnumValuesDefinition, err = p.parseEnumValuesDefinition()
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	p.putEnumTypeDefinition(definition)

	return nil
}
