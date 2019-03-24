package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

func (p *Parser) parseInlineFragment(startPosition position.Position) (ref int, err error) {

	var fragment document.InlineFragment
	p.initInlineFragment(&fragment)
	fragment.Position.MergeStartIntoStart(startPosition)

	hasTypeCondition := p.peekExpect(keyword.ON, true)
	if hasTypeCondition {
		fragmentIdent, err := p.readExpect(keyword.IDENT, "parseInlineFragment")
		if err != nil {
			return ref, err
		}
		fragmentType := p.makeType(&fragment.TypeCondition)
		fragmentType.Name = fragmentIdent.Literal
		fragmentType.Kind = document.TypeKindNAMED
		p.putType(fragmentType, fragment.TypeCondition)
	}

	err = p.parseDirectives(&fragment.DirectiveSet)
	if err != nil {
		return ref, err
	}

	err = p.parseSelectionSet(&fragment.SelectionSet)
	if err != nil {
		return ref, err
	}

	fragment.Position.MergeStartIntoEnd(p.TextPosition())
	return p.putInlineFragment(fragment), err
}
