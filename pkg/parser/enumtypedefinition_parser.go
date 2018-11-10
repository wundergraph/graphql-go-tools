package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseEnumTypeDefinition() (enumTypeDefinition document.EnumTypeDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	enumTypeDefinition.Name = string(tok.Literal)

	enumTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	enumTypeDefinition.EnumValuesDefinition, err = p.parseEnumValuesDefinition()

	return
}
