package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseSelectionSet() (selectionSet document.SelectionSet, err error) {

	if _, matched, err := p.readOptionalToken(token.CURLYBRACKETOPEN); err != nil || !matched {
		return selectionSet, err
	}

	for {

		tok, err := p.read(WithReadRepeat())
		if err != nil {
			return selectionSet, err
		}

		if tok.Keyword == token.CURLYBRACKETCLOSE || tok.Keyword == token.EOF {
			_, err = p.read()
			return selectionSet, err
		}

		selection, err := p.parseSelection()
		if err != nil {
			return selectionSet, err
		}
		selectionSet = append(selectionSet, selection)
	}

}
