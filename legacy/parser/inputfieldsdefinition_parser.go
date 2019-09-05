package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

func (p *Parser) parseInputFieldsDefinition(index *int) (err error) {

	if open := p.peekExpect(keyword.LBRACE, false); !open {
		return nil
	}

	start := p.l.Read()

	var definition document.InputFieldsDefinition
	definition.Position.MergeStartIntoStart(start.TextPosition)

	definition.InputValueDefinitions, err = p.parseInputValueDefinitions(keyword.RBRACE)
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.RBRACE, "parseInputFieldsDefinition")
	if err != nil {
		return
	}

	definition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = p.putInputFieldsDefinitions(definition)
	return
}
