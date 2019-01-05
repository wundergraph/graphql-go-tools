package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFragmentSpread(index *[]int) error {

	fragmentSpread := p.makeFragmentSpread()

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentSpread")
	if err != nil {
		return err
	}

	fragmentSpread.FragmentName = fragmentIdent.Literal
	err = p.parseDirectives(&fragmentSpread.Directives)
	if err != nil {
		return err
	}

	*index = append(*index, p.putFragmentSpread(fragmentSpread))

	return nil
}
