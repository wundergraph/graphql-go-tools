package parser

/*func (p *Parser) parseListType(index *int) error {

	_, err := p.readExpect(keyword.SQUAREBRACKETOPEN, "parseListType")
	if err != nil {
		return err
	}

	listType := p.makeType(index)

	var ofTypeIndex int

	err = p.parseType(&ofTypeIndex)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.SQUAREBRACKETCLOSE, "parseListType")
	if err != nil {
		return err
	}

	isNonNull, err := p.peekExpect(keyword.BANG, true)
	if err != nil {
		return err
	}

	if isNonNull {

	}

	return nil
}
*/
