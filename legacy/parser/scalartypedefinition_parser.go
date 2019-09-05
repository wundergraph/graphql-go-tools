package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseScalarTypeDefinition(hasDescription, isExtend bool, description token.Token) error {

	start, err := p.readExpect(keyword.SCALAR, "parseScalarTypeDefinition")
	if err != nil {
		return err
	}

	scalar, err := p.readExpect(keyword.IDENT, "parseScalarTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeScalarTypeDefinition()
	definition.Name = scalar.Literal
	definition.IsExtend = true

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

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	p.putScalarTypeDefinition(definition)

	return nil
}
