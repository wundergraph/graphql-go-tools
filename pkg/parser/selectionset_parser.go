package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSelectionSet(set *document.SelectionSet) (err error) {

	hasSubSelection, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return err
	}

	if !hasSubSelection {
		return
	}

	for {

		next := p.l.Peek(true)

		if next == keyword.CURLYBRACKETCLOSE {
			p.l.Read()
			return nil
		}

		isFragmentSelection, err := p.peekExpect(keyword.SPREAD, true)
		if err != nil {
			return err
		}

		if !isFragmentSelection {
			err := p.parseField(&set.Fields)
			if err != nil {
				return err
			}
		} else {

			isInlineFragment, err := p.peekExpect(keyword.ON, true)
			if err != nil {
				return err
			}

			if isInlineFragment {

				err := p.parseInlineFragment(&set.InlineFragments)
				if err != nil {
					return err
				}

			} else {

				err := p.parseFragmentSpread(&set.FragmentSpreads)
				if err != nil {
					return err
				}
			}
		}
	}
}
