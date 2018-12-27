package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseScalarTypeDefinition() (scalarTypeDefinition document.ScalarTypeDefinition, err error) {

	scalar, err := p.readExpect(keyword.IDENT, "parseScalarTypeDefinition")
	if err != nil {
		return
	}

	scalarTypeDefinition.Name = scalar.Literal

	scalarTypeDefinition.Directives, err = p.parseDirectives()

	return
}
