package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentsDefinition(index *int) (err error) {

	start, matches := p.peekExpectSwallow(keyword.BRACKETOPEN)
	if !matches {
		return nil
	}

	var definition document.ArgumentsDefinition
	definition.Position.MergeStartIntoStart(start.TextPosition)

	definition.InputValueDefinitions, err = p.parseInputValueDefinitions(keyword.BRACKETCLOSE)
	if err != nil {
		return
	}

	end, err := p.readExpect(keyword.BRACKETCLOSE, "parseArgumentsDefinition")
	definition.Position.MergeEndIntoEnd(end.TextPosition)

	*index = p.putArgumentsDefinition(definition)

	return err
}
