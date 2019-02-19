package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

func (p *Parser) parseFragmentSpread(startPosition position.Position) (ref int, err error) {

	fragmentSpread := p.makeFragmentSpread()
	fragmentSpread.Position.MergeStartIntoStart(startPosition)

	fragmentIdent, err := p.readExpect(keyword.IDENT, "parseFragmentSpread")
	if err != nil {
		return ref, err
	}

	fragmentSpread.FragmentName = p.putByteSliceReference(fragmentIdent.Literal)
	err = p.parseDirectives(&fragmentSpread.DirectiveSet)
	if err != nil {
		return ref, err
	}

	fragmentSpread.Position.MergeStartIntoEnd(p.TextPosition())

	return p.putFragmentSpread(fragmentSpread), err
}
