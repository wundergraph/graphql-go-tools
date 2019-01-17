package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseDirectiveDefinition(index *[]int) error {

	_, err := p.readExpect(keyword.AT, "parseDirectiveDefinition")
	if err != nil {
		return err
	}

	directiveIdent, err := p.readExpect(keyword.IDENT, "parseDirectiveDefinition")
	if err != nil {
		return err
	}

	definition := p.makeDirectiveDefinition()

	definition.Name = directiveIdent.Literal

	err = p.parseArgumentsDefinition(&definition.ArgumentsDefinition)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.ON, "parseDirectiveDefinition")
	if err != nil {
		return err
	}

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return err
		}

		if next == keyword.PIPE {
			_, err = p.l.Read()
			if err != nil {
				return err
			}
		} else if next == keyword.IDENT {
			location, err := p.l.Read()
			if err != nil {
				return err
			}

			parsedLocation, err := document.ParseDirectiveLocation(p.ByteSlice(location.Literal))
			if err != nil {
				return err
			}

			definition.DirectiveLocations = append(definition.DirectiveLocations, parsedLocation)

		} else {
			break
		}
	}

	*index = append(*index, p.putDirectiveDefinition(definition))

	return nil
}
