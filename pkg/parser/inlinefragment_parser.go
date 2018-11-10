package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseInlineFragment() (inlineFragment document.InlineFragment, err error) {

	tok, matched, err := p.readOptionalToken(token.IDENT)
	if err != nil {
		return
	}

	if matched {
		inlineFragment.TypeCondition = document.NamedType{
			Name: string(tok.Literal),
		}
	}

	inlineFragment.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	inlineFragment.SelectionSet, err = p.parseSelectionSet()

	return
}
