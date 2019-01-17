package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedFloatValue(index *int) error {

	floatToken, err := p.l.Read()
	if err != nil {
		return err
	}

	float, err := transform.StringToFloat32(p.ByteSlice(floatToken.Literal))
	if err != nil {
		return err
	}

	*index = p.putFloat(float)
	return nil
}
