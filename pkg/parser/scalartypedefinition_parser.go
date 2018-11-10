package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseScalarTypeDefinition() (scalarTypeDefinition document.ScalarTypeDefinition, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return
	}

	scalarTypeDefinition.Name = string(tok.Literal)

	scalarTypeDefinition.Directives, err = p.parseDirectives()

	return
}
