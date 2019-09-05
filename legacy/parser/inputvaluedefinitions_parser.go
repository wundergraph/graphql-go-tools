package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputValueDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions(closeKeyword keyword.Keyword) (definitions document.InputValueDefinitions, err error) {

	var hasDescription bool
	var description token.Token
	nextRef := -1

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.STRING, keyword.COMMENT:
			quote := p.l.Read()
			description = quote
			hasDescription = true
		case keyword.IDENT, keyword.TYPE, keyword.MUTATION:
			ident := p.l.Read()
			definition := p.makeInputValueDefinition()

			if hasDescription {
				definition.Description = description.Literal
				definition.Position.MergeStartIntoStart(description.TextPosition)
				hasDescription = false
			} else {
				definition.Position.MergeStartIntoStart(ident.TextPosition)
			}

			definition.Name = ident.Literal

			_, err = p.readExpect(keyword.COLON, "parseInputValueDefinitions")
			if err != nil {
				return
			}

			err = p.parseType(&definition.Type)
			if err != nil {
				return
			}

			definition.DefaultValue, err = p.parseDefaultValue()
			if err != nil {
				return
			}

			err = p.parseDirectives(&definition.DirectiveSet)
			if err != nil {
				return
			}

			definition.Position.MergeStartIntoEnd(p.TextPosition())
			definition.NextRef = nextRef
			nextRef = p.putInputValueDefinition(definition)
		default:
			if next != closeKeyword && closeKeyword != keyword.UNDEFINED {
				invalid := p.l.Read()
				err = newErrInvalidType(invalid.TextPosition, "parseInputValueDefinitions", "string/ident/"+closeKeyword.String(), invalid.String())
				return
			} else {
				return document.NewInputValueDefinitions(nextRef), err
			}
		}
	}
}
