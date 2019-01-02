package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseEnumValuesDefinition() (values document.EnumValuesDefinition, err error) {

	hasCurlyBracketOpen, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return values, err
	}

	if !hasCurlyBracketOpen {
		return
	}

	var description string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return values, err
		}

		if next == keyword.STRING {

			stringToken, err := p.l.Read()
			if err != nil {
				return values, err
			}

			description = stringToken.Literal
			continue

		} else if next == keyword.IDENT {
			ident, err := p.l.Read()
			if err != nil {
				return values, err
			}

			enumValueDefinition := document.EnumValueDefinition{
				EnumValue:   ident.Literal,
				Description: description,
			}

			description = ""

			enumValueDefinition.Directives, err = p.parseDirectives()
			if err != nil {
				return values, err
			}

			values = append(values, enumValueDefinition)
			continue

		} else if next == keyword.CURLYBRACKETCLOSE {
			_, err = p.l.Read()
			return values, err
		}

		invalid, _ := p.l.Read()
		return values, newErrInvalidType(invalid.Position, "parseEnumValuesDefinition", "string/ident/curlyBracketClose", invalid.Keyword.String())
	}
}
