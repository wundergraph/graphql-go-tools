package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSchemaDefinition() (schemaDefinition document.SchemaDefinition, err error) {

	schemaDefinition.Directives, err = p.parseDirectives()
	if err != nil {
		return
	}

	_, err = p.readExpect(keyword.CURLYBRACKETOPEN, "parseSchemaDefinition")
	if err != nil {
		return
	}

	for {
		next, err := p.l.Read()
		if err != nil {
			return schemaDefinition, err
		}

		switch next.Keyword {
		case keyword.CURLYBRACKETCLOSE:
			return schemaDefinition, err
		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			_, err = p.readExpect(keyword.COLON, "parseSchemaDefinition")
			if err != nil {
				return schemaDefinition, err
			}

			operationNameToken, err := p.readExpect(keyword.IDENT, "parseSchemaDefinition")
			if err != nil {
				return schemaDefinition, err
			}

			err = schemaDefinition.SetOperationType(next.Literal, operationNameToken.Literal)

			if err != nil {
				return schemaDefinition, err
			}

		default:
			return schemaDefinition, newErrInvalidType(next.Position, "parseSchemaDefinition", "curlyBracketClose/query/subscription/mutation", next.String())
		}
	}
}
