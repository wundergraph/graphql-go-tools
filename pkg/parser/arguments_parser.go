package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseArguments() (arguments document.Arguments, err error) {

	key, err := p.l.Peek(true)
	if err != nil {
		return nil, err
	}

	if key != keyword.BRACKETOPEN {
		return
	}

	_, err = p.l.Read()
	if err != nil {
		return
	}

	var valueName []byte

	for {
		key, err = p.l.Peek(true)
		if err != nil {
			return nil, err
		}

		if key == keyword.IDENT {
			identToken, err := p.l.Read()
			if err != nil {
				return nil, err
			}

			valueName = identToken.Literal

		} else if key == keyword.BRACKETCLOSE {
			_, err = p.l.Read()
			return arguments, err
		} else {
			return nil, fmt.Errorf("parseArguments: ident/bracketclose expected, got %s", key)
		}

		key, err = p.l.Peek(true)
		if err != nil {
			return nil, err
		}

		if key == keyword.COLON {
			_, err = p.l.Read()
			if err != nil {
				return
			}
		} else {
			return nil, fmt.Errorf("parseArguments: colon expected, got %s", key)
		}

		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}

		arguments = append(arguments, document.Argument{
			Name:  valueName,
			Value: value,
		})
	}
}
