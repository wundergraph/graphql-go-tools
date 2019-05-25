package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseTypeSystemDefinition() (err error) {

	var hasDescription bool
	var isExtend bool
	var description token.Token

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
			p.l.Read()
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

			if p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition.IsDefined() {
				invalid := p.l.Read()
				return newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "not a re-assignment of SchemaDefinition", "multiple SchemaDefinition assignments")
			}

			err = p.parseSchemaDefinition(&p.ParsedDefinitions.TypeSystemDefinition.SchemaDefinition)
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

			err := p.parseUnionTypeDefinition(hasDescription, description)
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
