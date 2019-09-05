package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentsDefinition(index *int) (err error) {

	start, matches := p.peekExpectSwallow(keyword.LPAREN)
	if !matches {
		return nil
	}

	var definition document.ArgumentsDefinition
	definition.Position.MergeStartIntoStart(start.TextPosition)

	definition.InputValueDefinitions, err = p.parseInputValueDefinitions(keyword.RPAREN)
	if err != nil {
		return
	}

	end, err := p.readExpect(keyword.RPAREN, "parseArgumentsDefinition")
	definition.Position.MergeEndIntoEnd(end.TextPosition)

	*index = p.putArgumentsDefinition(definition)

	return err
}
