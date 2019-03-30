package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseEnumValuesDefinition(index *[]int) error {

	if open := p.peekExpect(keyword.CURLYBRACKETOPEN, true); !open {
		return nil
	}

	var hasDescription bool
	var description document.ByteSliceReference

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

			err := p.parseDirectives(&definition.DirectiveSet)
			if err != nil {
				return err
			}

			*index = append(*index, p.putEnumValueDefinition(definition))
			continue

		} else if next == keyword.CURLYBRACKETCLOSE {
			p.l.Read()
			return nil
		}

		invalid := p.l.Read()
		return newErrInvalidType(invalid.TextPosition, "parseEnumValuesDefinition", "string/ident/curlyBracketClose", invalid.Keyword.String())
	}
}
