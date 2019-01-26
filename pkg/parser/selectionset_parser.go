package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSelectionSet(set *document.SelectionSet) (err error) {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, false); !open {
		return
	}

	start := p.l.Read()
	set.Position.MergeStartIntoStart(start.TextPosition)

	for {

		next := p.l.Peek(true)

		if next == keyword.EOF {
			return fmt.Errorf("parseSelectionSet: unexpected EOF, forgot to close bracket? ")
		} else if next == keyword.CURLYBRACKETCLOSE {
			end := p.l.Read()
			set.Position.MergeEndIntoEnd(end.TextPosition)
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

			isFragmentSpread := p.peekExpect(keyword.IDENT, false)
			if isFragmentSpread {
				err := p.parseFragmentSpread(start.TextPosition, &set.FragmentSpreads)
				if err != nil {
					return err
				}
			} else {
				err := p.parseInlineFragment(start.TextPosition, &set.InlineFragments)
				if err != nil {
					return err
				}
			}
		}
	}
}
