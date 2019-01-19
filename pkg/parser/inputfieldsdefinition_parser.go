package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputFieldsDefinition(index *[]int) error {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, true); !open {
		return nil
	}

	err := p.parseInputValueDefinitions(index, keyword.CURLYBRACKETCLOSE)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.CURLYBRACKETCLOSE, "parseInputFieldsDefinition")
	return err
}
