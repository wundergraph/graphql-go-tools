package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseTypeSystemDefinition() (definition document.TypeSystemDefinition, err error) {

	p.initTypeSystemDefinition(&definition)

	var hasDescription bool
	var description token.Token

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.EOF:
			return definition, err
		case keyword.STRING:
			descriptionToken := p.l.Read()
			description = descriptionToken
			hasDescription = true
			continue
		case keyword.SCHEMA:

			if definition.SchemaDefinition.IsDefined() {
				invalid := p.l.Read()
				return definition, newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "not a re-assignment of SchemaDefinition", "multiple SchemaDefinition assignments")
			}

			err = p.parseSchemaDefinition(&definition.SchemaDefinition)
			if err != nil {
				return definition, err
			}

		case keyword.SCALAR:

			err := p.parseScalarTypeDefinition(hasDescription, description, &definition.ScalarTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.TYPE:

			err := p.parseObjectTypeDefinition(hasDescription, description, &definition.ObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.INTERFACE:

			err := p.parseInterfaceTypeDefinition(hasDescription, description, &definition.InterfaceTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.UNION:

			err := p.parseUnionTypeDefinition(hasDescription, description, &definition.UnionTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.ENUM:

			err := p.parseEnumTypeDefinition(hasDescription, description, &definition.EnumTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.INPUT:

			err := p.parseInputObjectTypeDefinition(hasDescription, description, &definition.InputObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.DIRECTIVE:

			err := p.parseDirectiveDefinition(hasDescription, description, &definition.DirectiveDefinitions)
			if err != nil {
				return definition, err
			}

		default:
			invalid := p.l.Read()
			return definition, newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "eof/string/schema/scalar/type/interface/union/directive/input/enum", invalid.Keyword.String())
		}

		hasDescription = false
	}
}
