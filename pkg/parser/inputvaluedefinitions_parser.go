package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputValueDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions(index *[]int, closeKeyword keyword.Keyword) error {

	var description *document.ByteSliceReference

	for {
		next := p.l.Peek(true)

		if next == keyword.STRING {

			quote := p.l.Read()

			//*description = transform.TrimWhitespace(p.ByteSlice(quote.Literal)) TODO: fix trimming
			description = &quote.Literal

		} else if next == keyword.IDENT {

			ident := p.l.Read()
			definition := p.makeInputValueDefinition()
			if description != nil {
				definition.Description = *description
			}
			definition.Name = ident.Literal

			description = nil

			_, err := p.readExpect(keyword.COLON, "parseInputValueDefinitions")
			if err != nil {
				return err
			}

			err = p.parseType(&definition.Type)
			if err != nil {
				return err
			}

			err = p.parseDefaultValue(&definition.DefaultValue)
			if err != nil {
				return err
			}

			err = p.parseDirectives(&definition.Directives)
			if err != nil {
				return err
			}

			*index = append(*index, p.putInputValueDefinition(definition))

		} else if next != closeKeyword && closeKeyword != keyword.UNDEFINED {
			invalid := p.l.Read()
			return newErrInvalidType(invalid.TextPosition, "parseInputValueDefinitions", "string/ident/"+closeKeyword.String(), invalid.String())
		} else {
			return nil
		}
	}
}
