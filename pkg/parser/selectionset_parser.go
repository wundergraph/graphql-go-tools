package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSelectionSet() (selectionSet document.SelectionSet, err error) {

	hasSubSelection, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return selectionSet, err
	}

	if !hasSubSelection {
		return
	}

	for {

		next, err := p.l.Peek(true)
		if err != nil {
			return selectionSet, err
		}

		if next == keyword.CURLYBRACKETCLOSE {
			_, err = p.l.Read()
			return selectionSet, err
		}

		selection, err := p.parseSelection()
		if err != nil {
			return selectionSet, err
		}

		selectionSet = append(selectionSet, selection)
	}

}
