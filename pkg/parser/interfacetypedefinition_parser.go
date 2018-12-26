package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInterfaceTypeDefinition() (interfaceTypeDefinition document.InterfaceTypeDefinition, err error) {

	interfaceName, err := p.readExpect(keyword.IDENT, "parseInterfaceTypeDefinition")
	if err != nil {
		return
	}

	interfaceTypeDefinition.Name = string(interfaceName.Literal)

	interfaceTypeDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	interfaceTypeDefinition.FieldsDefinition, err = p.parseFieldsDefinition()

	return
}
