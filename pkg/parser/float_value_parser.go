package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedFloatValue(index *int) error {

	floatToken := p.l.Read()

	float, err := transform.StringToFloat32(p.ByteSlice(floatToken.Literal))
	if err != nil {
		return err
	}

	*index = p.putFloat(float)
	return nil
}
