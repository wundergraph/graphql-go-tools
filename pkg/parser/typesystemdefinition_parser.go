package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/token"
)

func (p *Parser) parseTypeSystemDefinition() (typeSystemDefinition document.TypeSystemDefinition, err error) {
	_, err = p.readAllUntil(token.EOF, WithDescription(), WithWhitelist(token.IDENT)).
		foreachMatchedPattern(Pattern(token.IDENT),
			func(tokens []token.Token) (err error) {

				identifier := tokens[0].Literal
				description := tokens[0].Description
				position := tokens[0].Position

				switch {
				case identifier.Equals(literal.SCHEMA):
					if typeSystemDefinition.SchemaDefinition.IsDefined() {
						return newErrInvalidType(position, "parseTypeSystemDefinition", "not a re-assignment of SchemaDefinition", "multiple SchemaDefinition assignments")
					}
					typeSystemDefinition.SchemaDefinition, err = p.parseSchemaDefinition()
					return err
				case identifier.Equals(literal.SCALAR):
					scalarTypeDefinition, err := p.parseScalarTypeDefinition()
					scalarTypeDefinition.Description = description
					typeSystemDefinition.ScalarTypeDefinitions = append(typeSystemDefinition.ScalarTypeDefinitions, scalarTypeDefinition)
					return err
				case identifier.Equals(literal.TYPE):
					objectTypeDefinition, err := p.parseObjectTypeDefinition()
					objectTypeDefinition.Description = description
					typeSystemDefinition.ObjectTypeDefinitions = append(typeSystemDefinition.ObjectTypeDefinitions, objectTypeDefinition)
					return err
				case identifier.Equals(literal.INTERFACE):
					interfaceTypeDefinition, err := p.parseInterfaceTypeDefinition()
					interfaceTypeDefinition.Description = description
					typeSystemDefinition.InterfaceTypeDefinitions = append(typeSystemDefinition.InterfaceTypeDefinitions, interfaceTypeDefinition)
					return err
				case identifier.Equals(literal.UNION):
					unionTypeDefinition, err := p.parseUnionTypeDefinition()
					unionTypeDefinition.Description = description
					typeSystemDefinition.UnionTypeDefinitions = append(typeSystemDefinition.UnionTypeDefinitions, unionTypeDefinition)
					return err
				case identifier.Equals(literal.ENUM):
					enumTypeDefinition, err := p.parseEnumTypeDefinition()
					enumTypeDefinition.Description = description
					typeSystemDefinition.EnumTypeDefinitions = append(typeSystemDefinition.EnumTypeDefinitions, enumTypeDefinition)
					return err
				case identifier.Equals(literal.INPUT):
					inputObjectTypeDefinition, err := p.parseInputObjectTypeDefinition()
					inputObjectTypeDefinition.Description = description
					typeSystemDefinition.InputObjectTypeDefinitions = append(typeSystemDefinition.InputObjectTypeDefinitions, inputObjectTypeDefinition)
					return err
				case identifier.Equals(literal.DIRECTIVE):
					directiveDefinition, err := p.parseDirectiveDefinition()
					directiveDefinition.Description = description
					typeSystemDefinition.DirectiveDefinitions = append(typeSystemDefinition.DirectiveDefinitions, directiveDefinition)
					return err
				default:
					return newErrInvalidType(position, "parseTypeSystemDefinition", "a valid TypeSystemDefinition identifier", string(identifier))
				}
			})
	return
}
