package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseImplementsInterfaces() (implementsInterfaces document.ImplementsInterfaces, err error) {

	if _, matched, err := p.readOptionalLiteral(literal.IMPLEMENTS); err != nil || !matched {
		return implementsInterfaces, err
	}

	tok, err := p.read(WithWhitelist(token.IDENT))
	if err != nil {
		return implementsInterfaces, err
	}

	implementsInterfaces = append(implementsInterfaces, string(tok.Literal))

	_, err = p.readAllUntil(token.EOF, WithReadRepeat()).
		foreachMatchedPattern(Pattern(token.AND, token.IDENT),
			func(tokens []token.Token) error {
				implementsInterfaces = append(implementsInterfaces, string(tokens[1].Literal))
				return nil
			})
	return
}
