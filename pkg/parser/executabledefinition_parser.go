package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	isSimpleQuery := p.peekExpect(keyword.CURLYBRACKETOPEN, false)

	if isSimpleQuery {
		return p.parseSimpleQueryExecutableDefinition()
	}

	return p.parseComplexExecutableDefinition()
}

func (p *Parser) parseSimpleQueryExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	executableDefinition = p.makeExecutableDefinition()

	var operationDefinition document.OperationDefinition
	p.initOperationDefinition(&operationDefinition)
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
		next := p.l.Peek(true)

		switch next {
		case keyword.FRAGMENT:

			p.l.Read()

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
				invalid := p.l.Read()
				err = newErrInvalidType(invalid.TextPosition, "parseComplexExecutableDefinition", "fragment/query/mutation/subscription", next.String())
			}

			return executableDefinition, err
		}
	}
}
