package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseVariableDefinitions(index *[]int) (err error) {

	if open := p.peekExpect(keyword.BRACKETOPEN, true); !open {
		return
	}

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.VARIABLE:

			variable := p.l.Read()

			variableDefinition := document.VariableDefinition{
				DefaultValue: -1,
				Variable:     p.putByteSliceReference(variable.Literal),
			}

			variableDefinition.Position.MergeStartIntoStart(variable.TextPosition)

			_, err = p.readExpect(keyword.COLON, "parseVariableDefinitions")
			if err != nil {
				return err
			}

			err = p.parseType(&variableDefinition.Type)
			if err != nil {
				return err
			}

			err = p.parseDefaultValue(&variableDefinition.DefaultValue)
			if err != nil {
				return err
			}

			variableDefinition.Position.MergeStartIntoEnd(p.TextPosition())
			*index = append(*index, p.putVariableDefinition(variableDefinition))

		case keyword.BRACKETCLOSE:
			p.l.Read()
			return err
		default:
			invalid := p.l.Read()
			return newErrInvalidType(invalid.TextPosition, "parseVariableDefinitions", "variable/bracket close", invalid.Keyword.String())
		}
	}
}
