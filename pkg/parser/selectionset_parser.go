package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSelectionSet(ref *int) (err error) {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, false); !open {
		return
	}

	start := p.l.Read()
	var set document.SelectionSet
	p.initSelectionSet(&set)
	set.Position.MergeStartIntoStart(start.TextPosition)

	for {

		next := p.l.Peek(true)

		if next == keyword.EOF {
			return fmt.Errorf("parseSelectionSet: unexpected EOF, forgot to close bracket? ")
		} else if next == keyword.CURLYBRACKETCLOSE {
			end := p.l.Read()
			set.Position.MergeEndIntoEnd(end.TextPosition)
			*ref = p.putSelectionSet(set)
			return nil
		}

		isFragmentSelection := p.peekExpect(keyword.SPREAD, false)
		if !isFragmentSelection {

			field, err := p.parseField()
			if err != nil {
				return err
			}

			set.Fields = append(set.Fields, field)

		} else {

			start := p.l.Read()

			isFragmentSpread := p.peekExpect(keyword.IDENT, false)
			if isFragmentSpread {

				fragmentSpread, err := p.parseFragmentSpread(start.TextPosition)
				if err != nil {
					return err
				}

				set.FragmentSpreads = append(set.FragmentSpreads, fragmentSpread)

			} else {

				inlineFragment, err := p.parseInlineFragment(start.TextPosition)
				if err != nil {
					return err
				}

				set.InlineFragments = append(set.InlineFragments, inlineFragment)
			}
		}
	}
}
