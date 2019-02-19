package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputValueDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions(index *[]int, closeKeyword keyword.Keyword) error {

	var description *token.Token

	for {
		next := p.l.Peek(true)

		if next == keyword.STRING {

			quote := p.l.Read()
			description = &quote

		} else if next == keyword.IDENT {

			ident := p.l.Read()
			definition := p.makeInputValueDefinition()

			if description != nil {
				definition.Description = description.Literal
				definition.Position.MergeStartIntoStart(description.TextPosition)
				description = nil
			} else {
				definition.Position.MergeStartIntoStart(ident.TextPosition)
			}

			definition.Name = p.putByteSliceReference(ident.Literal)

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

			err = p.parseDirectives(&definition.DirectiveSet)
			if err != nil {
				return err
			}

			definition.Position.MergeStartIntoEnd(p.TextPosition())
			*index = append(*index, p.putInputValueDefinition(definition))

		} else if next != closeKeyword && closeKeyword != keyword.UNDEFINED {
			invalid := p.l.Read()
			return newErrInvalidType(invalid.TextPosition, "parseInputValueDefinitions", "string/ident/"+closeKeyword.String(), invalid.String())
		} else {
			return nil
		}
	}
}
