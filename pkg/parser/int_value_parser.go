package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/transform"
)

func (p *Parser) parsePeekedIntValue(index *int) error {

	integerToken, err := p.l.Read()
	if err != nil {
		return err
	}

	integer, err := transform.StringToInt32(p.ByteSlice(integerToken.Literal))
	if err != nil {
		return err
	}

	*index = p.putInteger(integer)
	return nil
}
