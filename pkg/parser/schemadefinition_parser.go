package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseSchemaDefinition() (schemaDefinition document.SchemaDefinition, err error) {

	schemaDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	_, err = p.read(WithWhitelist(token.CURLYBRACKETOPEN))
	if err != nil {
		return
	}

	_, err = p.readAllUntil(token.CURLYBRACKETCLOSE,
		WithWhitelist(token.COLON, token.IDENT)).
		foreachMatchedPattern(Pattern(token.IDENT, token.COLON, token.IDENT),
			func(tokens []token.Token) error {

				operationType := string(tokens[0].Literal)
				operationName := string(tokens[2].Literal)

				return schemaDefinition.SetOperationType(operationType, operationName)
			})

	return schemaDefinition, err
}
