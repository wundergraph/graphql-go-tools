package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputValueDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions(index *[]int, closeKeyword keyword.Keyword) error {

	var description string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return err
		}

		if next == keyword.STRING {

			quote, err := p.l.Read()
			if err != nil {
				return err
			}

			description = transform.TrimWhitespace(quote.Literal)

		} else if next == keyword.IDENT {

			ident, err := p.l.Read()
			if err != nil {
				return err
			}

			definition := p.makeInputValueDefinition()
			definition.Description = description
			definition.Name = ident.Literal

			description = ""

			_, err = p.readExpect(keyword.COLON, "parseInputValueDefinitions")
			if err != nil {
				return err
			}

			definition.Type, err = p.parseType()
			if err != nil {
				return err
			}
			definition.DefaultValue, err = p.parseDefaultValue()
			if err != nil {
				return err
			}

			err = p.parseDirectives(&definition.Directives)
			if err != nil {
				return err
			}

			*index = append(*index, p.putInputValueDefinition(definition))

		} else if next != closeKeyword && closeKeyword != keyword.UNDEFINED {
			invalid, _ := p.l.Read()
			return newErrInvalidType(invalid.Position, "parseInputValueDefinitions", "string/ident/"+closeKeyword.String(), invalid.String())
		} else { // nolint
			return nil
		}
	}
}
