package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseInlineFragment(index *[]int) error {

	fragment := p.makeInlineFragment()

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseInlineFragment")
	if err != nil {
		return err
	}

	fragment.TypeCondition = document.NamedType{
		Name: fragmentIdent.Literal,
	}

	err = p.parseDirectives(&fragment.Directives)
	if err != nil {
		return err
	}

	err = p.parseSelectionSet(&fragment.SelectionSet)
	if err != nil {
		return err
	}

	*index = append(*index, p.putInlineFragment(fragment))
	return nil
}
