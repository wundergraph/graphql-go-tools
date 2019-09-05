package parser

import "github.com/jensneuse/graphql-go-tools/legacy/document"

func (p *Parser) parsePeekedByteSlice(value *document.Value) {
	variableToken := p.l.Read()
	value.Raw = variableToken.Literal
	value.Reference = p._putByteSliceReference(variableToken.Literal)
}
