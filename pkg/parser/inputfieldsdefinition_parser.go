package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputFieldsDefinition() (inputFieldsDefinition document.InputFieldsDefinition, err error) {

	hasFields, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return inputFieldsDefinition, err
	}

	if !hasFields {
		return
	}

	inputFieldsDefinition, err = p.parseInputValueDefinitions()
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.CURLYBRACKETCLOSE, "parseInputFieldsDefinition")

	return
}
