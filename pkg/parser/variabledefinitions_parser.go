package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseVariableDefinitions() (variableDefinitions document.VariableDefinitions, err error) {

	hasVariableDefinitions, err := p.peekExpect(keyword.BRACKETOPEN, true)
	if err != nil {
		return variableDefinitions, err
	}

	if !hasVariableDefinitions {
		return
	}

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return variableDefinitions, err
		}

		switch next {
		case keyword.VARIABLE:

			variable, err := p.l.Read()
			if err != nil {
				return variableDefinitions, err
			}

			variableDefinition := document.VariableDefinition{
				Variable: string(variable.Literal),
			}

			_, err = p.readExpect(keyword.COLON, "parseVariableDefinitions")
			if err != nil {
				return variableDefinitions, err
			}

			variableDefinition.Type, err = p.parseType()
			if err != nil {
				return variableDefinitions, err
			}

			variableDefinition.DefaultValue, err = p.parseDefaultValue()
			if err != nil {
				return variableDefinitions, err
			}

			variableDefinitions = append(variableDefinitions, variableDefinition)

		case keyword.BRACKETCLOSE:
			_, err = p.l.Read()
			return variableDefinitions, err
		default:
			invalid, _ := p.l.Read()
			return variableDefinitions, newErrInvalidType(invalid.Position, "parseVariableDefinitions", "variable/bracket close", invalid.Keyword.String())
		}
	}
}
