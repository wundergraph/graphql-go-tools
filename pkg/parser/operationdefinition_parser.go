package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseOperationDefinition(index *[]int) (err error) {

	var operationDefinition document.OperationDefinition
	p.initOperationDefinition(&operationDefinition)

	operationType := p.l.Peek(true)

	switch operationType {
	case keyword.QUERY:
		operationDefinition.OperationType = document.OperationTypeQuery
		p.l.Read()
	case keyword.MUTATION:
		operationDefinition.OperationType = document.OperationTypeMutation
		p.l.Read()
	case keyword.SUBSCRIPTION:
		operationDefinition.OperationType = document.OperationTypeSubscription
		p.l.Read()
	default:
		operationDefinition.OperationType = document.OperationTypeQuery
	}

	isNamedOperation := p.peekExpect(keyword.IDENT, false)
	if isNamedOperation {
		name := p.l.Read()
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

	*index = append(*index, p.putOperationDefinition(operationDefinition))

	return
}
