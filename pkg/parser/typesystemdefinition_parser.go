package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseTypeSystemDefinition() (definition document.TypeSystemDefinition, err error) {

	definition = p.makeTypeSystemDefinition()

	var description *token.Token

	for {
		next := p.l.Peek(true)

		switch next {
		case keyword.EOF:
			return definition, err
		case keyword.STRING:
			descriptionToken := p.l.Read()
			description = &descriptionToken
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

			err := p.parseScalarTypeDefinition(description, &definition.ScalarTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.TYPE:

			err := p.parseObjectTypeDefinition(description, &definition.ObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.INTERFACE:

			err := p.parseInterfaceTypeDefinition(description, &definition.InterfaceTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.UNION:

			err := p.parseUnionTypeDefinition(description, &definition.UnionTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.ENUM:

			err := p.parseEnumTypeDefinition(description, &definition.EnumTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.INPUT:

			err := p.parseInputObjectTypeDefinition(description, &definition.InputObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

		case keyword.DIRECTIVE:

			err := p.parseDirectiveDefinition(description, &definition.DirectiveDefinitions)
			if err != nil {
				return definition, err
			}

		default:
			invalid := p.l.Read()
			return definition, newErrInvalidType(invalid.TextPosition, "parseTypeSystemDefinition", "eof/string/schema/scalar/type/interface/union/directive/input/enum", invalid.Keyword.String())
		}

		description = nil
	}
}
