package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInputFieldsDefinition() (inputFieldsDefinition document.InputFieldsDefinition, err error) {

	if _, matched, err := p.readOptionalToken(token.CURLYBRACKETOPEN); err != nil || !matched {
		return inputFieldsDefinition, err
	}

	inputFieldsDefinition, err = p.parseInputValueDefinitions()
	if err != nil {
		return
	}

	_, err = p.read(WithWhitelist(token.CURLYBRACKETCLOSE))
	if err != nil {
		return
	}

	return
}
