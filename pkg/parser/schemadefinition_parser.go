package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSchemaDefinition() (definition document.SchemaDefinition, err error) {

	definition.Directives = p.indexPoolGet()

	err = p.parseDirectives(&definition.Directives)
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
			return definition, err
		}

		switch next.Keyword {
		case keyword.CURLYBRACKETCLOSE:
			return definition, err
		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			_, err = p.readExpect(keyword.COLON, "parseSchemaDefinition")
			if err != nil {
				return definition, err
			}

			operationNameToken, err := p.readExpect(keyword.IDENT, "parseSchemaDefinition")
			if err != nil {
				return definition, err
			}

			err = definition.SetOperationType(next.Literal, operationNameToken.Literal)

			if err != nil {
				return definition, err
			}

		default:
			return definition, newErrInvalidType(next.Position, "parseSchemaDefinition", "curlyBracketClose/query/subscription/mutation", next.String())
		}
	}
}
