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

	var valueName string

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

		value, err := p.parseValue()
		if err != nil {
			return err
		}

		argument := document.Argument{
			Name:  valueName,
			Value: value,
		}

		*index = append(*index, p.putArgument(argument))
	}
}
