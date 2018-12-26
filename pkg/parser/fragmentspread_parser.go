package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFragmentSpread() (fragmentSpread document.FragmentSpread, err error) {

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentSpread")
	if err != nil {
		return fragmentSpread, err
	}

	fragmentSpread.FragmentName = string(fragmentIdent.Literal)
	fragmentSpread.Directives, err = p.parseDirectives()
	return
}
