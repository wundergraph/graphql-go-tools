package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseEnumTypeDefinition() (enumTypeDefinition document.EnumTypeDefinition, err error) {

	ident, err := p.readExpect(keyword.IDENT, "parseEnumTypeDefinition")
	if err != nil {
		return
	}

	enumTypeDefinition.Name = string(ident.Literal)

	enumTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	enumTypeDefinition.EnumValuesDefinition, err = p.parseEnumValuesDefinition()

	return
}
