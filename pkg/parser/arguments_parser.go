package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArguments(index *[]int) error {

	key, err := p.l.Peek(true)
	if err != nil {
		return err
	}

	if key != keyword.BRACKETOPEN {
		return nil
	}

	_, err = p.l.Read()
	if err != nil {
		return err
	}

	var valueName document.ByteSliceReference

	for {
		key, err = p.l.Peek(true)
		if err != nil {
			return err
		}

		if key == keyword.IDENT {
			identToken, err := p.l.Read()
			if err != nil {
				return err
			}

			valueName = identToken.Literal

		} else if key == keyword.BRACKETCLOSE {
			_, err = p.l.Read()
			return err
		} else {
			return fmt.Errorf("parseArguments: ident/bracketclose expected, got %s", key)
		}

		key, err = p.l.Peek(true)
		if err != nil {
			return err
		}

		if key == keyword.COLON {
			_, err = p.l.Read()
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("parseArguments: colon expected, got %s", key)
		}

		argument := document.Argument{
			Name: valueName,
		}

		err := p.parseValue(&argument.Value)
		if err != nil {
			return err
		}

		*index = append(*index, p.putArgument(argument))
	}
}
