package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentsDefinition(index *[]int) error {

	isBracketOpen, err := p.peekExpect(keyword.BRACKETOPEN, true)
	if err != nil {
		return err
	}

	if !isBracketOpen {
		return nil
	}

	err = p.parseInputValueDefinitions(index, keyword.BRACKETCLOSE)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.BRACKETCLOSE, "parseArgumentsDefinition")
	return err
}
