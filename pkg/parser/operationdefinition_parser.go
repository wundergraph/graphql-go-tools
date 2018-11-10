package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseOperationDefinition() (operationDefinition document.OperationDefinition, err error) {

	tok, matched, err := p.readOptionalToken(token.IDENT)
	if err != nil {
		return
	}
	if matched {
		operationDefinition.Name = string(tok.Literal)
	}

	operationDefinition.VariableDefinitions, err = p.parseVariableDefinitions()
	if err != nil {
		return
	}

	operationDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	tok, err = p.read(WithWhitelist(token.CURLYBRACKETOPEN), WithReadRepeat())
	if err != nil {
		return
	}

	operationDefinition.SelectionSet, err = p.parseSelectionSet()

	return
}
