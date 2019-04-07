package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseTypeSystemDefinition(t *testing.T) {
	t.Run("unions", func(t *testing.T) {
		run(`
				"unifies SearchResult"
				union SearchResult = Photo | Person
				union thirdUnion 
				"second union"
				union secondUnion
				union firstUnion @fromTop(to: "bottom")
				"unifies UnionExample"
				union UnionExample = First | Second
				`,
			mustParseTypeSystemDefinition(
				node(
					hasUnionTypeSystemDefinitions(
						node(
							hasName("SearchResult"),
							hasPosition(position.Position{
								LineStart: 2,
								CharStart: 5,
								LineEnd:   3,
								CharEnd:   40,
							}),
						),
						node(
							hasName("thirdUnion"),
						),
						node(
							hasName("secondUnion"),
						),
						node(
							hasName("firstUnion"),
						),
						node(
							hasName("UnionExample"),
							hasPosition(position.Position{
								LineStart: 8,
								CharStart: 5,
								LineEnd:   9,
								CharEnd:   40,
							}),
						),
					),
				),
			),
		)
	})
	t.Run("schema", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					}
					
					#this is a scalar
					scalar JSON

					"this is a Person"
					type Person {
						name: String
					}


					"describes firstEntity"
					interface firstEntity {
						name: String
					}

					"describes direction"
					enum Direction {
						NORTH
					}

					"describes Person"
					input Person {
						name: String
					}

					"describes someway"
					directive @ someway on SUBSCRIPTION | MUTATION`,
			mustParseTypeSystemDefinition(
				node(
					hasSchemaDefinition(
						hasPosition(position.Position{
							LineStart: 1,
							CharStart: 2,
							LineEnd:   4,
							CharEnd:   7,
						}),
					),
					hasScalarTypeSystemDefinitions(
						node(
							hasDescription("#this is a scalar"),
							hasName("JSON"),
							hasPosition(position.Position{
								LineStart: 6,
								CharStart: 6,
								LineEnd:   7,
								CharEnd:   17,
							}),
						),
					),
					hasObjectTypeSystemDefinitions(
						node(
							hasDescription("this is a Person"),
							hasName("Person"),
							hasPosition(position.Position{
								LineStart: 9,
								CharStart: 6,
								LineEnd:   12,
								CharEnd:   7,
							}),
						),
					),
					hasInterfaceTypeSystemDefinitions(
						node(
							hasName("firstEntity"),
							hasPosition(position.Position{
								LineStart: 15,
								CharStart: 6,
								LineEnd:   18,
								CharEnd:   7,
							}),
						),
					),
					hasEnumTypeSystemDefinitions(
						node(
							hasName("Direction"),
							hasPosition(position.Position{
								LineStart: 20,
								CharStart: 6,
								LineEnd:   23,
								CharEnd:   7,
							}),
						),
					),
					hasInputObjectTypeSystemDefinitions(
						node(
							hasName("Person"),
							hasPosition(position.Position{
								LineStart: 25,
								CharStart: 6,
								LineEnd:   28,
								CharEnd:   7,
							}),
						),
					),
					hasDirectiveDefinitions(
						node(
							hasName("someway"),
							hasPosition(position.Position{
								LineStart: 30,
								CharStart: 6,
								LineEnd:   31,
								CharEnd:   52,
							}),
						),
					),
				),
			))
	})
	t.Run("set schema multiple times", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					}

					schema {
						query: Query
						mutation: Mutation
					}`,
			mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid schema", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
					)`,
			mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid scalar", func(t *testing.T) {
		run(`scalar JSON @foo(bar: .)`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid object type definition", func(t *testing.T) {
		run(`type Foo implements Bar { foo(bar: .): Baz}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid interface type definition", func(t *testing.T) {
		run(`interface Bar { baz: [Bal!}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid union type definition", func(t *testing.T) {
		run(`union Foo = Bar | Baz | 1337`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid union type definition 2", func(t *testing.T) {
		run(`union Foo = Bar | Baz | "Bal"`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid enum type definition", func(t *testing.T) {
		run(`enum Foo { Bar @baz(bal: .)}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid input object type definition", func(t *testing.T) {
		run(`input Foo { foo(bar: .): Baz}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid directive definition", func(t *testing.T) {
		run(`directive @ foo ON InvalidLocation`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid directive definition 2", func(t *testing.T) {
		run(`directive @ foo(bar: .) ON QUERY`, mustPanic(mustParseTypeSystemDefinition(node())))
	})
	t.Run("invalid keyword", func(t *testing.T) {
		run(`unknown {}`, mustPanic(mustParseTypeSystemDefinition(node())))
	})

	// type system definition extension
	t.Run("extend with scalars", func(t *testing.T) {
		run(`schema {}`,
			mustPanic(mustParseTypeSystemDefinition(
				node(
					hasScalarTypeSystemDefinitions(
						node(
							hasName("String"),
						),
					),
				),
			)),
			mustExtendTypeSystemDefinition(`
			scalar String`, node(
				hasScalarTypeSystemDefinitions(
					node(
						hasName("String"),
					),
				),
			)),
			mustExtendTypeSystemDefinition(`
			scalar JSON`, node(
				hasScalarTypeSystemDefinitions(
					node(
						hasName("String"),
					),
					node(
						hasName("JSON"),
					),
				),
			)),
		)
	})
	t.Run("extend after setting executable definition should fail", func(t *testing.T) {
		run(`schema {}`,
			mustParseTypeSystemDefinition(
				node(),
			),
			mustParseAddedExecutableDefinition("{foo}", nil, nil),
			mustPanic(mustExtendTypeSystemDefinition(`
			scalar String`, node(
				hasScalarTypeSystemDefinitions(
					node(
						hasName("String"),
					),
				),
			))),
		)
	})
	t.Run("extend after setting executable definition should fail reverse", func(t *testing.T) {
		run(`schema {}`,
			mustParseTypeSystemDefinition(
				node(),
			),
			mustExtendTypeSystemDefinition(`
			scalar String`, node(
				hasScalarTypeSystemDefinitions(
					node(
						hasName("String"),
					),
				),
			)),
		)
	})
}
