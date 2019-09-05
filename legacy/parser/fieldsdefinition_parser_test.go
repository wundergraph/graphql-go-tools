package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseFieldsDefinition(t *testing.T) {
	t.Run("simple field definition", func(t *testing.T) {
		run(`{ name: String }`,
			mustParseFieldsDefinition(
				node(
					hasName("name"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 3,
						LineEnd:   1,
						CharEnd:   15,
					}),
				),
			))
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`{
					name: String
					age: Int
				}`,
			mustParseFieldsDefinition(
				node(
					hasName("age"),
					nodeType(
						hasTypeName("Int"),
					),
				),
				node(
					hasName("name"),
					nodeType(
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 6,
						LineEnd:   2,
						CharEnd:   18,
					}),
				),
			))
	})
	t.Run("with description", func(t *testing.T) {
		run(`{
					"describes the name"
					name: String
				}`,
			mustParseFieldsDefinition(
				node(
					hasDescription("describes the name"),
					hasName("name"),
				),
			))
	})
	t.Run("non null list", func(t *testing.T) {
		run(`{
					name: [ String ]!
					age: Int!
				}`,
			mustParseFieldsDefinition(
				node(
					hasName("age"),
				),
				node(
					hasName("name"),
					nodeType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindLIST),
						),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 6,
						LineEnd:   2,
						CharEnd:   23,
					}),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`{ name(foo: .): String }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`{ name. String }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`{ name: [String! }`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`{ name: String @foo(bar: .)}`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`{ name: String`,
			mustPanic(
				mustParseFieldsDefinition(
					node(
						hasName("name"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				)))
	})
}
