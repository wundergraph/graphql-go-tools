package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInlineFragment() (inlineFragment document.InlineFragment, err error) {

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseInlineFragment")
	if err != nil {
		return inlineFragment, err
	}

	inlineFragment.TypeCondition = document.NamedType{
		Name: string(fragmentIdent.Literal),
	}

	inlineFragment.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	inlineFragment.SelectionSet, err = p.parseSelectionSet()

	return
}
