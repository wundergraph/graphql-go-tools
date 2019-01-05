package parser

/*func (p *Parser) parseSelection(set *document.SelectionSet) error {

	isFragmentSelection, err := p.peekExpect(keyword.SPREAD, true)
	if err != nil {
		return err
	}

	if !isFragmentSelection {
		return p.parseField(&set.Fields)
	}

	isInlineFragment, err := p.peekExpect(keyword.ON, true)
	if err != nil {
		return err
	}

	if isInlineFragment {
		return p.parseInlineFragment(&set.InlineFragments)
	}

	return p.parseFragmentSpread(&set.FragmentSpreads)
}*/
