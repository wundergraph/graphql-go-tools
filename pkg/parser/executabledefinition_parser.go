package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseExecutableDefinition() (executableDefinition document.ExecutableDefinition, err error) {

	matched, err := p.readAllUntil(token.EOF, WithReadRepeat()).
		foreachMatchedPattern(Pattern(token.IDENT),
			func(tokens []token.Token) (err error) {

				identifier := tokens[0].Literal
				position := tokens[0].Position

				switch {
				case identifier.Equals(literal.FRAGMENT):
					fragmentDefinition, err := p.parseFragmentDefinition()
					executableDefinition.FragmentDefinitions = append(executableDefinition.FragmentDefinitions, fragmentDefinition)
					return err
				case identifier.Equals(literal.QUERY), identifier.Equals(literal.MUTATION), identifier.Equals(literal.SUBSCRIPTION):
					operationDefinition, err := p.parseOperationDefinition()
					if err != nil {
						return err
					}
					operationDefinition.OperationType, err = document.ParseOperationType(string(identifier))
					executableDefinition.OperationDefinitions = append(executableDefinition.OperationDefinitions, operationDefinition)
					return err
				default:
					return newErrInvalidType(position, "parseExecutableDefinition", "a valid ExecutableDefinition identifier", string(identifier))
				}
			})

	if err == nil && matched == 0 {
		operationDefinition, err := p.parseOperationDefinition()
		if err != nil {
			return executableDefinition, err
		}
		operationDefinition.OperationType = document.OperationTypeQuery
		executableDefinition.OperationDefinitions = append(executableDefinition.OperationDefinitions, operationDefinition)
	}

	return executableDefinition, err
}
