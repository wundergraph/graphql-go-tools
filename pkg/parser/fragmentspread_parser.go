package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseFragmentSpread() (fragmentSpread document.FragmentSpread, err error) {

	tok, err := p.read(WithWhitelist(token.IDENT), WithExcludeLiteral(literal.ON))
	if err != nil {
		return
	}

	fragmentSpread.FragmentName = string(tok.Literal)

	fragmentSpread.Directives, err = p.parseDirectives()

	return
}
