package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseExecutableDefinition(t *testing.T) {
	t.Run("query with variable, directive and field", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) { name }`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasDirectives(
							node(hasName("rename")),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
						hasPosition(position.Position{
							LineStart: 1,
							CharStart: 1,
							LineEnd:   1,
							CharEnd:   59,
						}),
					),
				),
			))
	})
	t.Run("mutation", func(t *testing.T) {
		run(`mutation allGophers`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeMutation),
						hasName("allGophers"),
					),
				),
			))
	})
	t.Run("subscription", func(t *testing.T) {
		run(`subscription allGophers`,
			mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeSubscription),
						hasName("allGophers"),
					),
				),
			))
	})
	t.Run("fragment with query", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}
				query Q1 {
					foo
				}
				`,
			mustParseExecutableDefinition(
				nodes(
					node(
						hasName("MyFragment"),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
				),
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("Q1"),
						hasFields(
							node(hasName("foo")),
						),
					),
				),
			))
	})
	t.Run("multiple queries", func(t *testing.T) {
		run(`
				query allGophers($color: String) {
					name
				}

				query allGophinas($color: String) {
					name
				}

				`,
			mustParseExecutableDefinition(nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophinas"),
						hasVariableDefinitions(
							node(
								hasName("color"),
								nodeType(
									hasTypeKind(document.TypeKindNAMED),
									hasTypeName("String"),
								),
							),
						),
						hasFields(
							node(hasName("name")),
						),
					),
				),
			))
	})
	t.Run("invalid", func(t *testing.T) {
		run(`
				Barry allGophers($color: String)@rename(index: 3) {
					name
				}`, mustParseExecutableDefinition(nil, nil))
	})
	t.Run("large nested object", func(t *testing.T) {
		run(`
				query QueryWithFragments {
					hero {
						...heroFields
					}
				}

				fragment heroFields on SuperHero {
					name
					skill
					...on DrivingSuperHero {
						vehicles {
							...vehicleFields
						}
					}
				}

				fragment vehicleFields on Vehicle {
					name
					weapon
				}
				`,
			mustParseExecutableDefinition(
				nodes(
					node(
						hasName("heroFields"),
						hasPosition(position.Position{
							LineStart: 8,
							CharStart: 5,
							LineEnd:   16,
							CharEnd:   6,
						}),
					),
					node(
						hasName("vehicleFields"),
						hasPosition(position.Position{
							LineStart: 18,
							CharStart: 5,
							LineEnd:   21,
							CharEnd:   6,
						}),
					),
				),
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("QueryWithFragments"),
						hasPosition(position.Position{
							LineStart: 2,
							CharStart: 5,
							LineEnd:   6,
							CharEnd:   6,
						}),
					),
				)))
	})
	t.Run("unnamed operation", func(t *testing.T) {
		run("{\n  hero {\n    id\n    name\n  }\n}\n",
			mustParseExecutableDefinition(nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(
							node(
								hasName("hero"),
								hasFields(
									node(
										hasName("id"),
									),
									node(
										hasName("name"),
									),
								),
							),
						),
					),
				),
			))
	})
	t.Run("unnamed operation with unclosed selection", func(t *testing.T) {
		run(`	{
						dog {
							...scalarSelectionsNotAllowedOnInt	
					}`,
			mustPanic(mustParseExecutableDefinition(nil, nil)))
	})
	t.Run("unnamed operation with fragment", func(t *testing.T) {
		run(`	{
						dog {
							...fieldNotDefined
						}
					}
					fragment fieldNotDefined on Pet {
  						meowVolume
					}`,
			mustParseExecutableDefinition(
				nodes(
					node(
						hasName("fieldNotDefined"),
						hasTypeName("Pet"),
					),
				),
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName(""),
					),
				),
			))
	})
	t.Run("invalid", func(t *testing.T) {
		run("{foo { bar(foo: .) }}",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("query SomeQuery {foo { bar(foo: .) }}",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("query SomeQuery {foo { bar }} fragment Fields on SomeQuery { foo(bar: .) }",
			mustPanic(mustParseExecutableDefinition(
				nil,
				nodes(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasFields(node(
							hasName("foo"),
							hasFields(node(
								hasName("\"bar\""),
							)),
						)),
					)),
			)))
	})
}
