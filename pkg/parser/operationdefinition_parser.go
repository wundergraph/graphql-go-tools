package parser

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseOperationDefinition(index *[]int) (err error) {

	var operationDefinition document.OperationDefinition
	p.initOperationDefinition(&operationDefinition)

	operationType, err := p.l.Peek(true)
	if err != nil {
		return err
	}

	switch operationType {
	case keyword.QUERY:
		operationDefinition.OperationType = document.OperationTypeQuery
		_, err = p.l.Read()
	case keyword.MUTATION:
		operationDefinition.OperationType = document.OperationTypeMutation
		_, err = p.l.Read()
	case keyword.SUBSCRIPTION:
		operationDefinition.OperationType = document.OperationTypeSubscription
		_, err = p.l.Read()
	default:
		operationDefinition.OperationType = document.OperationTypeQuery
	}

	if err != nil {
		return err
	}

	isNamedOperation, err := p.peekExpect(keyword.IDENT, false)
	if err != nil {
		return err
	}

	if isNamedOperation {
		name, err := p.l.Read()
		if err != nil {
			return err
		}
		operationDefinition.Name = name.Literal
	}

	err = p.parseVariableDefinitions(&operationDefinition.VariableDefinitions)
	if err != nil {
		return
	}

	err = p.parseDirectives(&operationDefinition.Directives)
	if err != nil {
		return
	}

	err = p.parseSelectionSet(&operationDefinition.SelectionSet)
	if operationDefinition.SelectionSet.IsEmpty() {
		err = fmt.Errorf("parseOperationDefinition: selectionSet must not be empty")
	}

	*index = append(*index, p.putOperationDefinition(operationDefinition))

	return
}
