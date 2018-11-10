package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseArguments() (arguments document.Arguments, err error) {

	if _, matched, err := p.readOptionalToken(token.BRACKETOPEN); err != nil || !matched {
		return arguments, err
	}

	_, err = p.readAllUntil(token.BRACKETCLOSE, WithReadRepeat()).
		foreachMatchedPattern(Pattern(token.IDENT, token.COLON),
			func(tokens []token.Token) error {
				value, err := p.parseValue()
				if err != nil {
					return err
				}
				arguments = append(arguments, document.Argument{
					Name:  string(tokens[0].Literal),
					Value: value,
				})
				return nil
			})

	_, err = p.read(WithWhitelist(token.BRACKETCLOSE))
	if err != nil {
		return
	}

	return
}
