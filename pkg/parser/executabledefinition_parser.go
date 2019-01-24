package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	executableDefinition = p.makeExecutableDefinition()

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.CURLYBRACKETOPEN:

			err := p.parseAnonymousOperation(&executableDefinition)
			if err != nil {
				return executableDefinition, err
			}

		case keyword.FRAGMENT:

			err := p.parseFragmentDefinition(&executableDefinition.FragmentDefinitions)
			if err != nil {
				return executableDefinition, err
			}

		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			err := p.parseOperationDefinition(&executableDefinition.OperationDefinitions)
			if err != nil {
				return executableDefinition, err
			}

		default:
			return
		}
	}
}

func (p *Parser) parseAnonymousOperation(executableDefinition *document.ExecutableDefinition) error {

	var operationDefinition document.OperationDefinition
	p.initOperationDefinition(&operationDefinition)
	operationDefinition.OperationType = document.OperationTypeQuery

	err := p.parseSelectionSet(&operationDefinition.SelectionSet)
	if err != nil {
		return err
	}

	executableDefinition.OperationDefinitions =
		append(executableDefinition.OperationDefinitions, p.putOperationDefinition(operationDefinition))

	return nil
}
