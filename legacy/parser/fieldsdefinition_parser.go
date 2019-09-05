package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

func (p *Parser) parseFieldDefinitions() (fieldDefinitions document.FieldDefinitions, err error) {

	if hasOpen := p.peekExpect(keyword.LBRACE, true); !hasOpen {
		return
	}

	var hasDescription bool
	var description document.ByteSliceReference
	var startPosition position.Position
	nextRef := -1

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.STRING, keyword.COMMENT:
			stringToken := p.l.Read()
			description = stringToken.Literal
			startPosition = stringToken.TextPosition
			hasDescription = true
		case keyword.RBRACE:
			p.l.Read()
			return document.NewFieldDefinitions(nextRef), err
		case keyword.IDENT, keyword.TYPE, keyword.MUTATION:

			fieldIdent := p.l.Read()
			definition := p.makeFieldDefinition()

			if hasDescription {
				definition.Description = description
				definition.Position.MergeStartIntoStart(startPosition)
				hasDescription = false
			} else {
				definition.Position.MergeStartIntoStart(fieldIdent.TextPosition)
			}

			definition.Name = fieldIdent.Literal

			err = p.parseArgumentsDefinition(&definition.ArgumentsDefinition)
			if err != nil {
				return
			}

			_, err = p.readExpect(keyword.COLON, "parseFieldDefinitions")
			if err != nil {
				return
			}

			err = p.parseType(&definition.Type)
			if err != nil {
				return
			}

			definition.Position.MergeStartIntoEnd(p.TextPosition())

			err = p.parseDirectives(&definition.DirectiveSet)
			if err != nil {
				return
			}

			definition.NextRef = nextRef
			nextRef = p.putFieldDefinition(definition)

		default:
			invalid := p.l.Read()
			err = newErrInvalidType(invalid.TextPosition, "parseFieldDefinitions", "string/curly bracket close/ident", invalid.Keyword.String())
			return
		}
	}
}
