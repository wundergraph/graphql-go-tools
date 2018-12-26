package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentsDefinition() (argumentsDefinition document.ArgumentsDefinition, err error) {

	isBracketOpen, err := p.peekExpect(keyword.BRACKETOPEN, true)
	if err != nil {
		return argumentsDefinition, err
	}

	if !isBracketOpen {
		return
	}

	argumentsDefinition, err = p.parseInputValueDefinitions()
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.BRACKETCLOSE, "parseArgumentsDefinition")
	return
}
