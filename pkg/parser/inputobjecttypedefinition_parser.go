package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputObjectTypeDefinition() (inputObjectTypeDefinition document.InputObjectTypeDefinition, err error) {

	ident, err := p.readExpect(keyword.IDENT, "parseInputObjectTypeDefinition")
	if err != nil {
		return
	}

	inputObjectTypeDefinition.Name = string(ident.Literal)

	inputObjectTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	inputObjectTypeDefinition.InputFieldsDefinition, err = p.parseInputFieldsDefinition()

	return
}
