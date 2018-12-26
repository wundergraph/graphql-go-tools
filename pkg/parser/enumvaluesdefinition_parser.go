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

			description = string(stringToken.Literal)

		} else if next == keyword.IDENT {
			ident, err := p.l.Read()
			if err != nil {
				return values, err
			}

			enumValueDefinition := document.EnumValueDefinition{
				EnumValue:   string(ident.Literal),
				Description: description,
			}

			description = ""

			enumValueDefinition.Directives, err = p.parseDirectives()
			if err != nil {
				return values, err
			}

			values = append(values, enumValueDefinition)

		} else if next == keyword.CURLYBRACKETCLOSE {
			_, err = p.l.Read()
			return values, err
		} else {
			invalid, _ := p.l.Read()
			err = newErrInvalidType(invalid.Position, "parseEnumValuesDefinition", "string/ident/curlyBracketClose", invalid.Keyword.String())
		}
	}
	/*
		_, err = p.readAllUntil(keyword.CURLYBRACKETCLOSE,
			WithWhitelist(keyword.IDENT, keyword.COLON),
			WithDescription(),
		).foreachMatchedPattern(Pattern(keyword.IDENT),
			func(tokens []token.Token) error {
				directives, err := p.parseDirectives()
				values = append(values, document.EnumValueDefinition{
					EnumValue:   string(tokens[0].Literal),
					Description: tokens[0].Description,
					Directives:  directives,
				})

				return err
			})

		return*/
}
