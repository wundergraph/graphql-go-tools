package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInterfaceTypeDefinition() (interfaceTypeDefinition document.InterfaceTypeDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	interfaceTypeDefinition.Name = string(tok.Literal)

	interfaceTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}
	interfaceTypeDefinition.FieldsDefinition, err = p.parseFieldsDefinition()

	return
}
