package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/token"
)

func (p *Parser) parseTypeSystemDefinition() (err error) {

	var hasDescription bool
	var isExtend bool
	var description token.Token
	var extendToken token.Token

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.EOF:
			return
		case keyword.EXTEND:

			if isExtend {
				invalid := p.l.Read()
				return newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "one of: schema,type,scalar,union,input", "extend")
			}

			isExtend = true
			extendToken = p.l.Read()
			continue

		case keyword.STRING, keyword.COMMENT:

			if isExtend {
				invalid := p.l.Read()
				return newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "one of: schema,type,scalar,union,input", "extend")
			}

			descriptionToken := p.l.Read()
			description = descriptionToken
			hasDescription = true
			continue

		case keyword.SCHEMA:

			err = p.parseSchemaDefinition(isExtend, extendToken)
			if err != nil {
				return err
			}

		case keyword.SCALAR:

			err := p.parseScalarTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.TYPE:

			err := p.parseObjectTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.INTERFACE:

			err := p.parseInterfaceTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.UNION:

			err := p.parseUnionTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.ENUM:

			err := p.parseEnumTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.INPUT:

			err := p.parseInputObjectTypeDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		case keyword.DIRECTIVE:

			err := p.parseDirectiveDefinition(hasDescription, isExtend, description)
			if err != nil {
				return err
			}

		default:
			invalid := p.l.Read()
			return newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "eof/string/schema/scalar/type/interface/union/directive/input/enum", invalid.Keyword.String())
		}

		hasDescription = false
		isExtend = false
	}
}
