package parser

func (p *Parser) parsePeekedByteSlice(index *int) error {

	variableToken, err := p.l.Read()
	if err != nil {
		return err
	}

	*index = p.putByteSlice(variableToken.Literal)
	return nil
}
