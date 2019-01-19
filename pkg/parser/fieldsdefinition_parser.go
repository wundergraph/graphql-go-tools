package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
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

	var description *document.ByteSliceReference

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.STRING:
			stringToken := p.l.Read()
			description = &stringToken.Literal

		case keyword.CURLYBRACKETCLOSE:
			p.l.Read()
			return nil
		case keyword.IDENT, keyword.TYPE:

			fieldIdent := p.l.Read()
			definition := p.makeFieldDefinition()
			if description != nil {
				definition.Description = *description
			}
			definition.Name = fieldIdent.Literal

			description = nil

			err = p.parseArgumentsDefinition(&definition.ArgumentsDefinition)
			if err != nil {
				return err
			}

			_, err = p.readExpect(keyword.COLON, "parseFieldsDefinition")
			if err != nil {
				return err
			}

			err = p.parseType(&definition.Type)
			if err != nil {
				return err
			}

			err = p.parseDirectives(&definition.Directives)
			if err != nil {
				return err
			}

			*index = append(*index, p.putFieldDefinition(definition))
		default:
			invalid := p.l.Read()
			return newErrInvalidType(invalid.TextPosition, "parseFieldsDefinition", "string/curly bracket close/ident", invalid.Keyword.String())
		}
	}
}
