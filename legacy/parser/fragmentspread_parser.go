package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
)

func (p *Parser) parseFragmentSpread(startPosition position.Position) (ref int, err error) {

	fragmentSpread := p.makeFragmentSpread()
	fragmentSpread.Position.MergeStartIntoStart(startPosition)

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentSpread")
	if err != nil {
		return ref, err
	}

	fragmentSpread.FragmentName = fragmentIdent.Literal
	err = p.parseDirectives(&fragmentSpread.DirectiveSet)
	if err != nil {
		return ref, err
	}

	fragmentSpread.Position.MergeStartIntoEnd(p.TextPosition())

	return p.putFragmentSpread(fragmentSpread), err
}
