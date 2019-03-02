package parser

import "github.com/jensneuse/graphql-go-tools/pkg/document"

func (p *Parser) parsePeekedByteSlice(value *document.Value) {
	variableToken := p.l.Read()
	value.Raw = variableToken.Literal
	value.Reference = p.putByteSliceReference(variableToken.Literal)
}
