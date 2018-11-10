package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseVariableDefinitions() (variableDefinitions document.VariableDefinitions, err error) {

	if _, matched, err := p.readOptionalToken(token.BRACKETOPEN); err != nil || !matched {
		return variableDefinitions, err
	}

	_, err = p.readAllUntil(token.BRACKETCLOSE, WithReadRepeat()).
		foreachMatchedPattern(Pattern(token.VARIABLE, token.COLON),
			func(tokens []token.Token) error {

				variableDefinition := document.VariableDefinition{
					Variable: string(tokens[0].Literal),
				}

				variableDefinition.Type, err = p.parseType()
				if err != nil {
					return err
				}

				variableDefinition.DefaultValue, err = p.parseDefaultValue()
				variableDefinitions = append(variableDefinitions, variableDefinition)

				return err
			})

	_, err = p.read(WithWhitelist(token.BRACKETCLOSE))
	if err != nil {
		return
	}

	return
}
