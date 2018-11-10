package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseFieldsDefinition() (fieldsDefinition document.FieldsDefinition, err error) {
	tok, err := p.read(WithReadRepeat())
	if tok.Keyword != token.CURLYBRACKETOPEN {
		return
	}

	_, err = p.read(WithWhitelist(token.CURLYBRACKETOPEN))
	if err != nil {
		return
	}

	_, err = p.readAllUntil(token.CURLYBRACKETCLOSE, WithReadRepeat(), WithDescription()).
		foreachMatchedPattern(Pattern(token.IDENT),
			func(tokens []token.Token) error {
				description := string(tokens[0].Description)
				name := string(tokens[0].Literal)
				argumentsDefinition, err := p.parseArgumentsDefinition()
				if err != nil {
					return err
				}
				_, err = p.read(WithWhitelist(token.COLON))
				if err != nil {
					return err
				}
				fieldType, err := p.parseType()
				if err != nil {
					return err
				}
				directives, err := p.parseDirectives()
				fieldsDefinition = append(fieldsDefinition, document.FieldDefinition{
					Description:         description,
					Name:                name,
					Type:                fieldType,
					ArgumentsDefinition: argumentsDefinition,
					Directives:          directives,
				})
				return err
			})

	_, err = p.read(WithWhitelist(token.CURLYBRACKETCLOSE))
	if err != nil {
		return
	}

	return
}
