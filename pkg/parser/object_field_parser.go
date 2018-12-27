package parser

import (
	// "fmt"
	document "github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseObjectField() (field document.ObjectField, err error) {

	ident, err := p.readExpect(keyword.IDENT, "parseObjectField")
	if err != nil {
		return field, err
	}

	field.Name = ident.Literal

	_, err = p.readExpect(keyword.COLON, "parseObjectField")
	if err != nil {
		return field, err
	}

	field.Value, err = p.parseValue()

	return
}
