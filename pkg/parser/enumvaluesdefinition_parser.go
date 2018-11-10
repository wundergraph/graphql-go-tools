package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseEnumValuesDefinition() (values document.EnumValuesDefinition, err error) {

	if _, matched, err := p.readOptionalToken(token.CURLYBRACKETOPEN); err != nil || !matched {
		return values, err
	}

	_, err = p.readAllUntil(token.CURLYBRACKETCLOSE,
		WithWhitelist(token.IDENT, token.COLON),
		WithDescription(),
	).foreachMatchedPattern(Pattern(token.IDENT),
		func(tokens []token.Token) error {
			directives, err := p.parseDirectives()
			values = append(values, document.EnumValueDefinition{
				EnumValue:   string(tokens[0].Literal),
				Description: tokens[0].Description,
				Directives:  directives,
			})

			return err
		})

	return
}
