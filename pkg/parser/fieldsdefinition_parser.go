package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseFieldsDefinition() (fieldsDefinition document.FieldsDefinition, err error) {

	hasSubFields, err := p.peekExpect(keyword.CURLYBRACKETOPEN, true)
	if err != nil {
		return fieldsDefinition, err
	}

	if !hasSubFields {
		return
	}

	var description string

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return fieldsDefinition, err
		}

		switch next {
		case keyword.STRING:
			stringToken, err := p.l.Read()
			if err != nil {
				return fieldsDefinition, err
			}

			description = stringToken.Literal

		case keyword.CURLYBRACKETCLOSE:
			_, err = p.l.Read()
			return fieldsDefinition, err
		case keyword.IDENT, keyword.TYPE:

			fieldIdent, err := p.l.Read()
			if err != nil {
				return fieldsDefinition, err
			}

			fieldDefinition := document.FieldDefinition{
				Description: description,
				Name:        fieldIdent.Literal,
			}

			description = ""

			fieldDefinition.ArgumentsDefinition, err = p.parseArgumentsDefinition()
			if err != nil {
				return fieldsDefinition, err
			}

			_, err = p.readExpect(keyword.COLON, "parseFieldsDefinition")
			if err != nil {
				return fieldsDefinition, err
			}

			fieldDefinition.Type, err = p.parseType()
			if err != nil {
				return fieldsDefinition, err
			}

			fieldDefinition.Directives, err = p.parseDirectives()
			if err != nil {
				return fieldsDefinition, err
			}

			fieldsDefinition = append(fieldsDefinition, fieldDefinition)
		default:
			invalid, _ := p.l.Read()
			return fieldsDefinition, newErrInvalidType(invalid.Position, "parseFieldsDefinition", "string/curly bracket close/ident", invalid.Keyword.String())
		}
	}
}
