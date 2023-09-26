package ast

import "github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/position"

type SchemaExtension struct {
	ExtendLiteral position.Position
	SchemaDefinition
}
