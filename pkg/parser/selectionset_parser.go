package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSelectionSet(set *document.SelectionSet) (err error) {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, true); !open {
		return
	}

	for {

		next := p.l.Peek(true)

		if next == keyword.CURLYBRACKETCLOSE {
			p.l.Read()
			return nil
		}

		isFragmentSelection := p.peekExpect(keyword.SPREAD, false)
		if !isFragmentSelection {
			err := p.parseField(&set.Fields)
			if err != nil {
				return err
			}
		} else {

			start := p.l.Read()

			isInlineFragment := p.peekExpect(keyword.ON, true)
			if isInlineFragment {

				err := p.parseInlineFragment(start.TextPosition, &set.InlineFragments)
				if err != nil {
					return err
				}

			} else {

				err := p.parseFragmentSpread(start.TextPosition, &set.FragmentSpreads)
				if err != nil {
					return err
				}
			}
		}
	}
}
