package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseScalarTypeDefinition(description *token.Token, index *[]int) error {

	start, err := p.readExpect(keyword.SCALAR, "parseScalarTypeDefinition")
	if err != nil {
		return err
	}

	scalar, err := p.readExpect(keyword.IDENT, "parseScalarTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeScalarTypeDefinition()
	definition.Name = p.putByteSliceReference(scalar.Literal)

	if description != nil {
		definition.Position.MergeStartIntoStart(description.TextPosition)
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putScalarTypeDefinition(definition))

	return nil
}
