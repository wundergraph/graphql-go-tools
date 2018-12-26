package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseOperationDefinition() (operationDefinition document.OperationDefinition, err error) {

	isNamedOperation, err := p.peekExpect(keyword.IDENT, false)
	if err != nil {
		return operationDefinition, err
	}

	if isNamedOperation {
		name, err := p.l.Read()
		if err != nil {
			return operationDefinition, err
		}
		operationDefinition.Name = string(name.Literal)
	}

	operationDefinition.VariableDefinitions, err = p.parseVariableDefinitions()
	if err != nil {
		return
	}

	operationDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	operationDefinition.SelectionSet, err = p.parseSelectionSet()
	if len(operationDefinition.SelectionSet) == 0 {
		err = fmt.Errorf("parseOperationDefinition: selectionSet must not be empty")
	}

	return
}
