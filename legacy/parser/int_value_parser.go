package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedIntValue(value *document.Value) error {

	integerToken := p.l.Read()

	integer, err := transform.StringToInt32(p.ByteSlice(integerToken.Literal))
	if err != nil {
		return err
	}

	value.Raw = integerToken.Literal
	value.Reference = p.putInteger(integer)

	return nil
}
