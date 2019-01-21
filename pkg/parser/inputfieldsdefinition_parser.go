package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInputFieldsDefinition(index *int) error {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, false); !open {
		return nil
	}

	start := p.l.Read()

	var definition document.InputFieldsDefinition
	p.initInputFieldsDefinition(&definition)
	definition.Position.MergeStartIntoStart(start.TextPosition)

	err := p.parseInputValueDefinitions(&definition.InputValueDefinitions, keyword.CURLYBRACKETCLOSE)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.CURLYBRACKETCLOSE, "parseInputFieldsDefinition")

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = p.putInputFieldsDefinitions(definition)

	return err
}
