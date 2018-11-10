package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseArgumentsDefinition() (argumentsDefinition document.ArgumentsDefinition, err error) {

	if _, matched, err := p.readOptionalToken(token.BRACKETOPEN); err != nil || !matched {
		return argumentsDefinition, err
	}

	argumentsDefinition, err = p.parseInputValueDefinitions()
	if err != nil {
		return
	}

	_, err = p.read(WithWhitelist(token.BRACKETCLOSE))
	if err != nil {
		return
	}

	return
}
