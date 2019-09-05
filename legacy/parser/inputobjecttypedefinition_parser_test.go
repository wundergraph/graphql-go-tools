package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseInputObjectTypeDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`input Person {
					name: String
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
							),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("multiple fields", func(t *testing.T) {
		run(`input Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(hasName("age")),
							node(hasName("name")),
						),
					),
				),
			),
		)
	})
	t.Run("with default value", func(t *testing.T) {
		run(`input Person {
					name: String = "Gophina"
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("input Person", mustParseInputObjectTypeDefinition(
			node(
				hasName("Person"),
			),
		))
	})
	t.Run("complex", func(t *testing.T) {
		run(`input Person @fromTop(to: "bottom") @fromBottom(to: "top"){
					name: String
				}`,
			mustParseInputObjectTypeDefinition(
				node(
					hasName("Person"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasInputFieldsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("input 1337 {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("input Person @foo(bar: .) {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("input Person { a: .}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("notinput Foo {}",
			mustPanic(
				mustParseInputObjectTypeDefinition(
					node(
						hasName("1337"),
					),
				)),
		)
	})
}
