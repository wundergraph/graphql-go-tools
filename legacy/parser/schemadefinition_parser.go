package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) parseSchemaDefinition(isExtend bool, extendToken token.Token) error {

	start, err := p.readExpect(keyword.SCHEMA, "parseSchemaDefinition")
	if err != nil {
		return err
	}

	definition := document.SchemaDefinition{
		DirectiveSet: -1,
		IsExtend:     isExtend,
	}

	if isExtend {
		definition.Position.MergeStartIntoStart(extendToken.TextPosition)
	} else {
		definition.Position.MergeStartIntoStart(start.TextPosition)
	}

	err = p.parseDirectives(&definition.DirectiveSet)
	if err != nil {
		return err
	}

	_, err = p.readExpect(keyword.LBRACE, "parseSchemaDefinition")
	if err != nil {
		return err
	}

	for {
		next := p.l.Read()

		switch next.Keyword {
		case keyword.RBRACE:
			definition.Position.MergeEndIntoEnd(next.TextPosition)
			p.putSchemaDefinition(definition)
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
