package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseTypeSystemDefinition() (definition document.TypeSystemDefinition, err error) {

	definition = p.makeTypeSystemDefinition()

	var description document.ByteSlice

	for {
		next, err := p.l.Read()
		if err != nil {
			return definition, err
		}

		switch next.Keyword {
		case keyword.EOF:
			return definition, err
		case keyword.STRING:

			description = next.Literal
			continue

		case keyword.SCHEMA:

			if definition.SchemaDefinition.IsDefined() {
				return definition, newErrInvalidType(next.Position, "parseTypeSystemDefinition", "not a re-assignment of SchemaDefinition", "multiple SchemaDefinition assignments")
			}

			definition.SchemaDefinition, err = p.parseSchemaDefinition()
			if err != nil {
				return definition, err
			}

		case keyword.SCALAR:

			err := p.parseScalarTypeDefinition(&definition.ScalarTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.ScalarTypeDefinitions[len(p.ParsedDefinitions.ScalarTypeDefinitions)-1].Description = description

		case keyword.TYPE:

			err := p.parseObjectTypeDefinition(&definition.ObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.ObjectTypeDefinitions[len(p.ParsedDefinitions.ObjectTypeDefinitions)-1].Description = description

		case keyword.INTERFACE:

			err := p.parseInterfaceTypeDefinition(&definition.InterfaceTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.InterfaceTypeDefinitions[len(p.ParsedDefinitions.InterfaceTypeDefinitions)-1].Description = description

		case keyword.UNION:

			err := p.parseUnionTypeDefinition(&definition.UnionTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.UnionTypeDefinitions[len(p.ParsedDefinitions.UnionTypeDefinitions)-1].Description = description

		case keyword.ENUM:

			err := p.parseEnumTypeDefinition(&definition.EnumTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.EnumTypeDefinitions[len(p.ParsedDefinitions.EnumTypeDefinitions)-1].Description =
				description

		case keyword.INPUT:

			err := p.parseInputObjectTypeDefinition(&definition.InputObjectTypeDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.InputObjectTypeDefinitions[len(p.ParsedDefinitions.InputObjectTypeDefinitions)-1].Description = description

		case keyword.DIRECTIVE:

			err := p.parseDirectiveDefinition(&definition.DirectiveDefinitions)
			if err != nil {
				return definition, err
			}

			p.ParsedDefinitions.DirectiveDefinitions[len(p.ParsedDefinitions.DirectiveDefinitions)-1].Description =
				description

		default:
			invalid, _ := p.l.Read()
			return definition, newErrInvalidType(invalid.Position, "parseTypeSystemDefinition", "eof/string/schema/scalar/type/interface/union/directive/input/enum", invalid.Keyword.String())
		}

		description = nil
	}
}
