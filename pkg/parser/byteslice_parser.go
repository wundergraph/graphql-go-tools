package parser

func (p *Parser) parsePeekedByteSlice(index *int) {
	variableToken := p.l.Read()
	*index = p.putByteSliceReference(variableToken.Literal)
}
