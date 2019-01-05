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

	executableDefinition = p.makeExecutableDefinition()

	operationDefinition := p.makeOperationDefinition()
	operationDefinition.OperationType = document.OperationTypeQuery

	err = p.parseSelectionSet(&operationDefinition.SelectionSet)
	if err != nil {
		return executableDefinition, err
	}

	executableDefinition.OperationDefinitions =
		append(executableDefinition.OperationDefinitions, p.putOperationDefinition(operationDefinition))

	return
}

func (p *Parser) parseComplexExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	executableDefinition = p.makeExecutableDefinition()

	for {
		next, err := p.l.Peek(true)
		if err != nil {
			return executableDefinition, err
		}

		switch next {
		case keyword.FRAGMENT:

			_, err = p.l.Read()
			if err != nil {
				return executableDefinition, err
			}

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

			if len(executableDefinition.OperationDefinitions) == 0 {
				invalid, _ := p.l.Read()
				err = newErrInvalidType(invalid.Position, "parseComplexExecutableDefinition", "fragment/query/mutation/subscription", next.String())
			}

			return executableDefinition, err
		}
	}
}
