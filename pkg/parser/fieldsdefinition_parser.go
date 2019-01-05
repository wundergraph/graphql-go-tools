package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFieldsDefinition(index *[]int) (err error) {

	hasSubFields, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return err
	}

	if !hasSubFields {
		return
	}

	var description string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return err
		}

		switch next {
		case keyword.STRING:
			stringToken, err := p.l.Read()
			if err != nil {
				return err
			}

			description = stringToken.Literal

		case keyword.CURLYBRACKETCLOSE:
			_, err = p.l.Read()
			return err
		case keyword.IDENT, keyword.TYPE:

			fieldIdent, err := p.l.Read()
			if err != nil {
				return err
			}

			definition := p.makeFieldDefinition()
			definition.Description = description
			definition.Name = fieldIdent.Literal

			description = ""

			err = p.parseArgumentsDefinition(&definition.ArgumentsDefinition)
			if err != nil {
				return err
			}

			_, err = p.readExpect(keyword.COLON, "parseFieldsDefinition")
			if err != nil {
				return err
			}

			definition.Type, err = p.parseType()
			if err != nil {
				return err
			}

			err = p.parseDirectives(&definition.Directives)
			if err != nil {
				return err
			}

			*index = append(*index, p.putFieldDefinition(definition))
		default:
			invalid, _ := p.l.Read()
			return newErrInvalidType(invalid.Position, "parseFieldsDefinition", "string/curly bracket close/ident", invalid.Keyword.String())
		}
	}
}
