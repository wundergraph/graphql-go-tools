package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputFieldsDefinition(index *int) (err error) {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, false); !open {
		return nil
	}

	start := p.l.Read()

	var definition document.InputFieldsDefinition
	definition.Position.MergeStartIntoStart(start.TextPosition)

	definition.InputValueDefinitions, err = p.parseInputValueDefinitions(keyword.CURLYBRACKETCLOSE)
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.CURLYBRACKETCLOSE, "parseInputFieldsDefinition")
	if err != nil {
		return
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = p.putInputFieldsDefinitions(definition)
	return
}
