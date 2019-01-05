package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseEnumValuesDefinition(index *[]int) error {

	hasCurlyBracketOpen, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return err
	}

	if !hasCurlyBracketOpen {
		return nil
	}

	var description string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return err
		}

		if next == keyword.STRING {

			stringToken, err := p.l.Read()
			if err != nil {
				return err
			}

			description = stringToken.Literal
			continue

		} else if next == keyword.IDENT {
			ident, err := p.l.Read()
			if err != nil {
				return err
			}

			definition := p.makeEnumValueDefinition()
			definition.EnumValue = ident.Literal
			definition.Description = description

			description = ""

			err = p.parseDirectives(&definition.Directives)
			if err != nil {
				return err
			}

			*index = append(*index, p.putEnumValueDefinition(definition))
			continue

		} else if next == keyword.CURLYBRACKETCLOSE {
			_, err = p.l.Read()
			return err
		}

		invalid, _ := p.l.Read()
		return newErrInvalidType(invalid.Position, "parseEnumValuesDefinition", "string/ident/curlyBracketClose", invalid.Keyword.String())
	}
}
