package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInputObjectTypeDefinition() (inputObjectTypeDefinition document.InputObjectTypeDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	inputObjectTypeDefinition.Name = string(tok.Literal)

	inputObjectTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	inputObjectTypeDefinition.InputFieldsDefinition, err = p.parseInputFieldsDefinition()

	return
}
