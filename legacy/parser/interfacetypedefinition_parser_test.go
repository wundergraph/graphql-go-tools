package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseInterfaceTypeDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`interface NameEntity {
					name: String
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("NameEntity"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
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
		run(`interface Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("Person"),
					hasFieldsDefinitions(
						node(
							hasName("age"),
						),
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run(`interface Person`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`interface NameEntity @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
			mustParseInterfaceTypeDefinition(
				node(
					hasName("NameEntity"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`interface 1337 {
					name: String
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("1337"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`interface Person @foo(bar: .) {
					name: String
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`interface Person {
					name: [String!
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(
					node(
						hasName("Person"),
						hasFieldsDefinitions(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`notinterface Person {
					name: [String!]
				}`,
			mustPanic(
				mustParseInterfaceTypeDefinition(),
			),
		)
	})
}
