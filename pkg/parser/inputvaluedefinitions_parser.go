package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

// InputValueDefinitions cannot be found in the graphQL spec.
// This parser was still implemented due to InputValueDefinition
// always appearing in a list, which may be contained by
// InputFieldsDefinitions: http://facebook.github.io/graphql/draft/#InputFieldsDefinition
// or ArgumentsDefinition http://facebook.github.io/graphql/draft/#ArgumentsDefinition

func (p *Parser) parseInputValueDefinitions() (inputValueDefinitions []document.InputValueDefinition, err error) {

	_, err = p.readAllUntil(token.EOF,
		WithReadRepeat(),
		WithDescription(),
	).foreachMatchedPattern(Pattern(token.IDENT, token.COLON),
		func(tokens []token.Token) error {
			ivdType, err := p.parseType()
			if err != nil {
				return err
			}
			defaultValue, err := p.parseDefaultValue()
			if err != nil {
				return err
			}
			directives, err := p.parseDirectives()
			inputValueDefinitions = append(inputValueDefinitions, document.InputValueDefinition{
				Description:  string(tokens[0].Description),
				Name:         string(tokens[0].Literal),
				Type:         ivdType,
				DefaultValue: defaultValue,
				Directives:   directives,
			})
			return err
		})

	return
}
