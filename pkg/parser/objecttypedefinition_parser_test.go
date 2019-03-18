package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseObjectTypeDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`type Person {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
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
		run(`type Person {
					name: [String]!
					age: [ Int ]
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
						node(
							hasName("age"),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run(`type Person`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
				),
			),
		)
	})
	t.Run("implements interface", func(t *testing.T) {
		run(`type Person implements Human {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasImplementsInterfaces("Human"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("implements multiple interfaces", func(t *testing.T) {
		run(`type Person implements Human & Mammal {
					name: String
				}`,
			mustParseObjectTypeDefinition(
				node(
					hasName("Person"),
					hasImplementsInterfaces("Human", "Mammal"),
					hasFieldsDefinitions(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`type Person @fromTop(to: "bottom") @fromBottom(to: "top") {
					name: String
				}`,
			mustParseObjectTypeDefinition(
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
		run(`type 1337 {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
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
		run(`type Person implements 1337 {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
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
		run(`type Person @foo(bar: .) {
					name: String
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
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
		run(`type Person {
					name: [String!
				}`,
			mustPanic(
				mustParseObjectTypeDefinition(
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
	t.Run("invalid 5", func(t *testing.T) {
		run(`nottype Person {
					name: [String!]
				}`,
			mustPanic(mustParseObjectTypeDefinition()),
		)
	})
}
