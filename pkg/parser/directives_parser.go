package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseDirectives() (directives document.Directives, err error) {
	_, err = p.readAllUntil(token.EOF, WithReadRepeat()).
		foreachMatchedPattern(Pattern(token.AT, token.IDENT),
			func(tokens []token.Token) error {
				arguments, err := p.parseArguments()
				if err != nil {
					return err
				}
				directives = append(directives, document.Directive{
					Name:      string(tokens[1].Literal),
					Arguments: arguments,
				})
				return nil
			})

	return
}
