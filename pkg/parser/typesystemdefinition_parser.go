package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
)

func (p *Parser) parseTypeSystemDefinition() (typeSystemDefinition document.TypeSystemDefinition, err error) {

	var description []byte

	for {
		next, err := p.l.Read()
		if err != nil {
			return typeSystemDefinition, err
		}

		switch next.Keyword {
		case keyword.EOF:
			return typeSystemDefinition, err
		case keyword.STRING:

			description = next.Literal
			continue

		case keyword.SCHEMA:

			if typeSystemDefinition.SchemaDefinition.IsDefined() {
				return typeSystemDefinition, newErrInvalidType(next.Position, "parseTypeSystemDefinition", "not a re-assignment of SchemaDefinition", "multiple SchemaDefinition assignments")
			}

			typeSystemDefinition.SchemaDefinition, err = p.parseSchemaDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

		case keyword.SCALAR:

			scalarTypeDefinition, err := p.parseScalarTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			scalarTypeDefinition.Description = description
			typeSystemDefinition.ScalarTypeDefinitions = append(typeSystemDefinition.ScalarTypeDefinitions, scalarTypeDefinition)

		case keyword.TYPE:

			objectTypeDefinition, err := p.parseObjectTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			objectTypeDefinition.Description = description
			typeSystemDefinition.ObjectTypeDefinitions = append(typeSystemDefinition.ObjectTypeDefinitions, objectTypeDefinition)

		case keyword.INTERFACE:

			interfaceTypeDefinition, err := p.parseInterfaceTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			interfaceTypeDefinition.Description = description
			typeSystemDefinition.InterfaceTypeDefinitions = append(typeSystemDefinition.InterfaceTypeDefinitions, interfaceTypeDefinition)

		case keyword.UNION:

			unionTypeDefinition, err := p.parseUnionTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			unionTypeDefinition.Description = description
			typeSystemDefinition.UnionTypeDefinitions = append(typeSystemDefinition.UnionTypeDefinitions, unionTypeDefinition)

		case keyword.ENUM:

			enumTypeDefinition, err := p.parseEnumTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			enumTypeDefinition.Description = description
			typeSystemDefinition.EnumTypeDefinitions = append(typeSystemDefinition.EnumTypeDefinitions, enumTypeDefinition)

		case keyword.INPUT:

			inputObjectTypeDefinition, err := p.parseInputObjectTypeDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			inputObjectTypeDefinition.Description = description
			typeSystemDefinition.InputObjectTypeDefinitions = append(typeSystemDefinition.InputObjectTypeDefinitions, inputObjectTypeDefinition)

		case keyword.DIRECTIVE:

			directiveDefinition, err := p.parseDirectiveDefinition()
			if err != nil {
				return typeSystemDefinition, err
			}

			directiveDefinition.Description = description
			typeSystemDefinition.DirectiveDefinitions = append(typeSystemDefinition.DirectiveDefinitions, directiveDefinition)

		default:
			invalid, _ := p.l.Read()
			return typeSystemDefinition, newErrInvalidType(invalid.Position, "parseTypeSystemDefinition", "eof/string/schema/scalar/type/interface/union/directive/input/enum", invalid.Keyword.String())
		}

		description = nil
	}
}
