package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArgumentsDefinition(index *int) error {

	start, matches := p.peekExpectSwallow(keyword.BRACKETOPEN)
	if !matches {
		return nil
	}

	var definition document.ArgumentsDefinition
	p.initArgumentsDefinition(&definition)
	definition.Position.MergeStartIntoStart(start.TextPosition)

	err := p.parseInputValueDefinitions(&definition.InputValueDefinitions, keyword.BRACKETCLOSE)
	if err != nil {
		return err
	}

	end, err := p.readExpect(keyword.BRACKETCLOSE, "parseArgumentsDefinition")
	definition.Position.MergeEndIntoEnd(end.TextPosition)

	*index = p.putArgumentsDefinition(definition)

	return err
}
