package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseSchemaDefinition(definition *document.SchemaDefinition) error {

	start, err := p.readExpect(keyword.SCHEMA, "parseSchemaDefinition")
	if err != nil {
		return err
	}

	definition.Position.MergeStartIntoStart(start.TextPosition)
	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.CURLYBRACKETOPEN, "parseSchemaDefinition")
	if err != nil {
		return err
	}

	for {
		next := p.l.Read()

		switch next.Keyword {
		case keyword.CURLYBRACKETCLOSE:
			definition.Position.MergeEndIntoEnd(next.TextPosition)
			return err
		case keyword.QUERY, keyword.MUTATION, keyword.SUBSCRIPTION:

			_, err = p.readExpect(keyword.COLON, "parseSchemaDefinition")
			if err != nil {
				return err
			}

			operationNameToken, err := p.readExpect(keyword.IDENT, "parseSchemaDefinition")
			if err != nil {
				return err
			}

			err = definition.SetOperationType(p.ByteSlice(next.Literal), operationNameToken.Literal)
			if err != nil {
				return err
			}

		default:
			return newErrInvalidType(next.TextPosition, "parseSchemaDefinition", "curlyBracketClose/query/subscription/mutation", next.String())
		}
	}
}
