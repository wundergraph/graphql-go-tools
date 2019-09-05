package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

func (p *Parser) parseOperationDefinition(index *[]int) (err error) {

	var operationDefinition document.OperationDefinition
	p.initOperationDefinition(&operationDefinition)

	operationType := p.l.Peek(true)

	switch operationType {
	case keyword.QUERY:
		operationDefinition.OperationType = document.OperationTypeQuery
		operationDefinition.Position.MergeStartIntoStart(p.l.Read().TextPosition)
	case keyword.MUTATION:
		operationDefinition.OperationType = document.OperationTypeMutation
		operationDefinition.Position.MergeStartIntoStart(p.l.Read().TextPosition)
	case keyword.SUBSCRIPTION:
		operationDefinition.OperationType = document.OperationTypeSubscription
		operationDefinition.Position.MergeStartIntoStart(p.l.Read().TextPosition)
	default:
		operationDefinition.OperationType = document.OperationTypeQuery
		operationDefinition.Position.MergeStartIntoStart(p.TextPosition())
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

	err = p.parseDirectives(&operationDefinition.DirectiveSet)
	if err != nil {
		return
	}

	err = p.parseSelectionSet(&operationDefinition.SelectionSet)

	operationDefinition.Position.MergeStartIntoEnd(p.TextPosition())
	*index = append(*index, p.putOperationDefinition(operationDefinition))

	return
}
