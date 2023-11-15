package ast

import "github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/position"

type SchemaExtension struct {
	ExtendLiteral position.Position
	SchemaDefinition
}
