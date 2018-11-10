package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseKeywordDividedIdentifiers(divider token.Keyword) (identifier []string, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT, divider))
	if err != nil {
		return identifier, err
	}

	if tok.Keyword == divider {

		tok, err = p.read(WithWhitelist(token.IDENT, divider))
		if err != nil {
			return identifier, err
		}

		identifier = append(identifier, string(tok.Literal))
	} else {
		identifier = append(identifier, string(tok.Literal))
	}

	for {
		if _, matched, err := p.readOptionalToken(divider); err != nil || !matched {
			return identifier, err
		}

		tok, err := p.read(WithWhitelist(token.IDENT))
		if err != nil {
			return identifier, err
		}

		identifier = append(identifier, string(tok.Literal))
	}

}
