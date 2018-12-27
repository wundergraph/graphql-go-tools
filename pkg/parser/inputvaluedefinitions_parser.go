package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputFieldsDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions() (inputValueDefinitions []document.InputValueDefinition, err error) {

	var description []byte

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return inputValueDefinitions, err
		}

		if next == keyword.STRING {

			quote, err := p.l.Read()
			if err != nil {
				return inputValueDefinitions, err
			}

			description = transform.TrimWhitespace(quote.Literal)

		} else if next == keyword.IDENT {

			ident, err := p.l.Read()
			if err != nil {
				return inputValueDefinitions, err
			}

			inputValueDefinition := document.InputValueDefinition{
				Description: description,
				Name:        ident.Literal,
			}

			description = nil

			_, err = p.readExpect(keyword.COLON, "parseInputValueDefinitions")
			if err != nil {
				return inputValueDefinitions, err
			}

			inputValueDefinition.Type, err = p.parseType()
			if err != nil {
				return inputValueDefinitions, err
			}
			inputValueDefinition.DefaultValue, err = p.parseDefaultValue()
			if err != nil {
				return inputValueDefinitions, err
			}

			inputValueDefinition.Directives, err = p.parseDirectives()
			if err != nil {
				return inputValueDefinitions, err
			}

			inputValueDefinitions = append(inputValueDefinitions, inputValueDefinition)

		} else {
			return inputValueDefinitions, nil
		}
	}
}
