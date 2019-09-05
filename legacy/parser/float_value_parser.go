package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedFloatValue(value *document.Value) error {

	floatToken := p.l.Read()

	float, err := transform.StringToFloat32(p.ByteSlice(floatToken.Literal))
	if err != nil {
		return err
	}

	value.Raw = floatToken.Literal
	value.Reference = p.putFloat(float)

	return nil
}
