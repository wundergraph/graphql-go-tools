package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	isSimpleQuery, err := p.peekExpect(keyword.CURLYBRACKETOPEN, false)
	if err != nil {
		return executableDefinition, err
	}

	if isSimpleQuery {
		return p.parseSimpleQueryExecutableDefinition()
	}

	return p.parseComplexExecutableDefinition()
}

func (p *Parser) parseSimpleQueryExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	operation := document.OperationDefinition{
		OperationType: document.OperationTypeQuery,
	}

	operation.SelectionSet, err = p.parseSelectionSet()
	if err != nil {
		return executableDefinition, err
	}

	executableDefinition.OperationDefinitions = make(document.OperationDefinitions, 1)
	executableDefinition.OperationDefinitions[0] = operation

	return
}

func (p *Parser) parseComplexExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	for {
		next, err := p.l.Read()
		if err != nil {
			return executableDefinition, err
		}

		switch next.Keyword {
		case keyword.FRAGMENT:

			fragmentDefinition, err := p.parseFragmentDefinition()
			if err != nil {
				return executableDefinition, err
			}

			executableDefinition.FragmentDefinitions = append(executableDefinition.FragmentDefinitions, fragmentDefinition)

		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			operationDefinition, err := p.parseOperationDefinition()
			if err != nil {
				return executableDefinition, err
			}

			if next.Keyword == keyword.QUERY {
				operationDefinition.OperationType = document.OperationTypeQuery
			} else if next.Keyword == keyword.MUTATION {
				operationDefinition.OperationType = document.OperationTypeMutation
			} else {
				operationDefinition.OperationType = document.OperationTypeSubscription
			}

			executableDefinition.OperationDefinitions = append(executableDefinition.OperationDefinitions, operationDefinition)

		default:

			if len(executableDefinition.OperationDefinitions) == 0 {
				err = newErrInvalidType(next.Position, "parseComplexExecutableDefinition", "fragment/query/mutation/subscription", next.String())
			}

			return executableDefinition, err
		}
	}
}
