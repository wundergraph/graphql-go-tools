package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
)

func (p *Parser) parseExecutableDefinition() (err error) {

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.LBRACE:

			err := p.parseAnonymousOperation(&p.ParsedDefinitions.ExecutableDefinition)
			if err != nil {
				return err
			}

		case keyword.FRAGMENT:

			err := p.parseFragmentDefinition(&p.ParsedDefinitions.ExecutableDefinition.FragmentDefinitions)
			if err != nil {
				return err
			}

		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			err := p.parseOperationDefinition(&p.ParsedDefinitions.ExecutableDefinition.OperationDefinitions)
			if err != nil {
				return err
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
