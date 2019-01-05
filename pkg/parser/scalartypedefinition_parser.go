package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseScalarTypeDefinition(index *[]int) error {

	scalar, err := p.readExpect(keyword.IDENT, "parseScalarTypeDefinition")
	if err != nil {
		return err
	}

	definition := p.makeScalarTypeDefinition()

	definition.Name = scalar.Literal

	err = p.parseDirectives(&definition.Directives)
	if err != nil {
		return err
	}

	*index = append(*index, p.putScalarTypeDefinition(definition))

	return nil
}
