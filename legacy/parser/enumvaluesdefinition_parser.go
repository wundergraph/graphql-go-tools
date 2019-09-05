package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

func (p *Parser) parseEnumValuesDefinition() (definitions document.EnumValueDefinitions, err error) {

	if open := p.peekExpect(keyword.LBRACE, true); !open {
		return document.NewEnumValueDefinitions(-1), err
	}

	var hasDescription bool
	var description document.ByteSliceReference
	nextRef := -1

	for {
		next := p.l.Peek(true)

		if next == keyword.STRING || next == keyword.COMMENT {

			stringToken := p.l.Read()
			description = stringToken.Literal
			hasDescription = true
			continue

		} else if next == keyword.IDENT {
			ident := p.l.Read()
			definition := p.makeEnumValueDefinition()
			definition.EnumValue = ident.Literal
			if hasDescription {
				definition.Description = description
				hasDescription = false
			}

			err = p.parseDirectives(&definition.DirectiveSet)
			if err != nil {
				return
			}

			definition.NextRef = nextRef
			nextRef = p.putEnumValueDefinition(definition)

			continue

		} else if next == keyword.RBRACE {
			p.l.Read()
			return document.NewEnumValueDefinitions(nextRef), err
		}

		invalid := p.l.Read()
		err = newErrInvalidType(invalid.TextPosition, "parseEnumValuesDefinition", "string/ident/curlyBracketClose", invalid.Keyword.String())
		return
	}
}
