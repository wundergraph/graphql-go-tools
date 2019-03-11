package parser

import (
	"encoding/json"
	"fmt"
	"github.com/jensneuse/diffview"
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"github.com/sebdah/goldie"
	"io/ioutil"
	"log"
	"testing"
)

func TestParser(t *testing.T) {

	// arguments

	t.Run("string argument", func(t *testing.T) {
		run(`(name: "Gophus")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("multiple argument sets", func(t *testing.T) {
		run(`(name: "Gophus")(name2: "Gophus")`,
			mustParseArguments(
				node(
					hasName("name"),
				),
			),
			mustParseArguments(
				node(
					hasName("name2"),
				),
			),
		)
	})
	t.Run("multiple argument sets", func(t *testing.T) {
		run(`(name: "Gophus")()`,
			mustParseArguments(
				node(
					hasName("name"),
				),
			),
			mustPanic(mustParseArguments(
				node(
					hasName("name2"),
				),
			)),
		)
	})
	t.Run("string array argument", func(t *testing.T) {
		run(`(fooBars: ["foo","bar"])`,
			mustParseArguments(
				node(
					hasName("fooBars"),
				),
			),
		)
	})
	t.Run("int array argument", func(t *testing.T) {
		run(`(integers: [1,2,3])`,
			mustParseArguments(
				node(
					hasName("integers"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   19,
					}),
				),
			),
		)
	})
	t.Run("multiple string arguments", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
				node(
					hasName("surname"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   39,
					}),
				),
			),
		)
	})
	t.Run("invalid argument must err", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson"`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 2", func(t *testing.T) {
		run(`((name: "Gophus", surname: "Gophersson")`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 3", func(t *testing.T) {
		run(`(name: .)`,
			mustPanic(mustParseArguments()))
	})

	// arguments definition

	t.Run("single int value", func(t *testing.T) {
		run(`(inputValue: Int)`,
			mustParseArgumentDefinition(
				node(
					hasInputValueDefinitions(
						node(
							hasName("inputValue"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   18,
					}),
				),
			),
		)
	})
	t.Run("optional value", func(t *testing.T) {
		run(" ", mustParseArgumentDefinition())
	})
	t.Run("multiple values", func(t *testing.T) {
		run(`(inputValue: Int, outputValue: String)`,
			mustParseArgumentDefinition(
				node(
					hasInputValueDefinitions(
						node(
							hasName("inputValue"),
						),
						node(
							hasName("outputValue"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   39,
					}),
				),
			),
		)
	})
	t.Run("not read optional", func(t *testing.T) {
		run(`inputValue: Int)`,
			mustParseArgumentDefinition())
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`((inputValue: Int)`,
			mustPanic(mustParseArgumentDefinition()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`(inputValue: Int`,
			mustPanic(mustParseArgumentDefinition()))
	})

	// parseDefaultValue

	t.Run("integer", func(t *testing.T) {
		run("= 2", mustParseDefaultValue(document.ValueTypeInt))
	})
	t.Run("bool", func(t *testing.T) {
		run("= true", mustParseDefaultValue(document.ValueTypeBoolean))
	})
	t.Run("invalid", func(t *testing.T) {
		run("true", mustPanic(mustParseDefaultValue(document.ValueTypeBoolean)))
	})

	// parseDirectiveDefinition

	t.Run("single directive with location", func(t *testing.T) {
		run("directive @ somewhere on QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   31,
					}),
				),
			),
		)
	})
	t.Run("trailing pipe", func(t *testing.T) {
		run("directive @ somewhere on | QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
				),
			),
		)
	})
	t.Run("with input value", func(t *testing.T) {
		run("directive @ somewhere(inputValue: Int) on QUERY",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY),
					hasArgumentsDefinition(
						hasInputValueDefinitions(
							node(
								hasName("inputValue"),
							),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   48,
					}),
				),
			),
		)
	})
	t.Run("multiple locations", func(t *testing.T) {
		run("directive @ somewhere on QUERY |\nMUTATION",
			mustParseDirectiveDefinition(
				node(
					hasName("somewhere"),
					hasDirectiveLocations(document.DirectiveLocationQUERY, document.DirectiveLocationMUTATION),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   2,
						CharStart: 1,
						CharEnd:   9,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("directive @ somewhere QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("directive @ somewhere off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("missing at", func(t *testing.T) {
		run("directive somewhere off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("invalid args", func(t *testing.T) {
		run("directive @ somewhere(inputValue: .) on QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("missing ident after at", func(t *testing.T) {
		run("directive @ \"somewhere\" off QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})
	t.Run("invalid location", func(t *testing.T) {
		run("directive @ somewhere on QUERY | thisshouldntwork",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
						hasDirectiveLocations(document.DirectiveLocationQUERY),
					),
				),
			),
		)
	})
	t.Run("invalid prefix", func(t *testing.T) {
		run("notdirective @ somewhere on QUERY",
			mustPanic(
				mustParseDirectiveDefinition(
					node(
						hasName("somewhere"),
					),
				),
			),
		)
	})

	// parseDirectives

	t.Run(`simple directive`, func(t *testing.T) {
		run(`@rename(index: 3)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   18,
					}),
				),
			),
		)
	})
	t.Run("multiple directives", func(t *testing.T) {
		run(`@rename(index: 3)@moveto(index: 4)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
				),
				node(
					hasName("moveto"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   35,
					}),
				),
			),
		)
	})
	t.Run("multiple arguments", func(t *testing.T) {
		run(`@rename(index: 3, count: 10)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
						node(
							hasName("count"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   29,
					}),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`@rename(index)`,
			mustPanic(mustParseDirectives()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`@1337(index)`,
			mustPanic(mustParseDirectives()),
		)
	})

	// parseEnumTypeDefinition

	t.Run("simple enum", func(t *testing.T) {
		run(`enum Direction {
						NORTH
						EAST
						SOUTH
						WEST
		}`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasEnumValuesDefinitions(
					node(hasName("NORTH")),
					node(hasName("EAST")),
					node(hasName("SOUTH")),
					node(hasName("WEST")),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   6,
					CharEnd:   4,
				}),
			),
		)
	})
	t.Run("enum with descriptions", func(t *testing.T) {
		run(`enum Direction {
  						"describes north"
  						NORTH
  						"describes east"
  						EAST
  						"describes south"
  						SOUTH
  						"describes west"
  						WEST }`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasEnumValuesDefinitions(
					node(hasName("NORTH"), hasDescription("describes north")),
					node(hasName("EAST"), hasDescription("describes east")),
					node(hasName("SOUTH"), hasDescription("describes south")),
					node(hasName("WEST"), hasDescription("describes west")),
				),
			))
	})
	t.Run("enum with space", func(t *testing.T) {
		run(`enum Direction {
  "describes north"
  NORTH

  "describes east"
  EAST

  "describes south"
  SOUTH

  "describes west"
  WEST
}`, mustParseEnumTypeDefinition(
			hasName("Direction"),
			hasEnumValuesDefinitions(
				node(hasName("NORTH"), hasDescription("describes north")),
				node(hasName("EAST"), hasDescription("describes east")),
				node(hasName("SOUTH"), hasDescription("describes south")),
				node(hasName("WEST"), hasDescription("describes west")),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   13,
				CharEnd:   2,
			}),
		))
	})
	t.Run("enum with directives", func(t *testing.T) {
		run(`enum Direction @fromTop(to: "bottom") @fromBottom(to: "top"){ NORTH }`,
			mustParseEnumTypeDefinition(
				hasName("Direction"),
				hasDirectives(
					node(hasName("fromTop")),
					node(hasName("fromBottom")),
				),
				hasEnumValuesDefinitions(
					node(hasName("NORTH")),
				),
			))
	})
	t.Run("enum without values", func(t *testing.T) {
		run("enum Direction", mustParseEnumTypeDefinition(hasName("Direction")))
	})
	t.Run("invalid enum", func(t *testing.T) {
		run("enum Direction {", mustPanic(mustParseEnumTypeDefinition()))
	})
	t.Run("invalid enum 2", func(t *testing.T) {
		run("enum  \"Direction\" {}", mustPanic(mustParseEnumTypeDefinition()))
	})
	t.Run("invalid enum 2", func(t *testing.T) {
		run("enum  Direction @from(foo: .)", mustPanic(mustParseEnumTypeDefinition(hasName("Direction"))))
	})
	t.Run("invalid enum 3", func(t *testing.T) {
		run("enum Direction {FOO @bar(baz: .)}", mustPanic(mustParseEnumTypeDefinition(hasName("Direction"))))
	})
	t.Run("invalid enum 4", func(t *testing.T) {
		run("notenum Direction", mustPanic(mustParseEnumTypeDefinition()))
	})

	// parseExecutableDefinition

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

	// parseField

	t.Run("parse field with name, arguments and directive", func(t *testing.T) {
		run("preferredName: originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   59,
					}),
				),
			),
		)
	})
	t.Run("without optional alias", func(t *testing.T) {
		run("originalName(isSet: true) @rename(index: 3)",
			mustParseFields(
				node(
					hasName("originalName"),
					hasArguments(
						node(hasName("isSet")),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional arguments", func(t *testing.T) {
		run("preferredName: originalName @rename(index: 3)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
				),
			),
		)
	})
	t.Run("without optional directives", func(t *testing.T) {
		run("preferredName: originalName(isSet: true)",
			mustParseFields(
				node(
					hasAlias("preferredName"),
					hasName("originalName"),
					hasArguments(
						node(
							hasName("isSet"),
						),
					),
				),
			),
		)
	})
	t.Run("with nested selection sets", func(t *testing.T) {
		run(`
				level1 {
					level2 {
						level3
					}
				}
				`,
			mustParseFields(
				node(
					hasName("level1"),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   6,
						CharEnd:   6,
					}),
					hasFields(
						node(
							hasName("level2"),
							hasPosition(position.Position{
								LineStart: 3,
								CharStart: 6,
								LineEnd:   5,
								CharEnd:   7,
							}),
							hasFields(
								node(
									hasName("level3"),
									hasPosition(position.Position{
										LineStart: 4,
										CharStart: 7,
										LineEnd:   4,
										CharEnd:   13,
									}),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`
				level1 {
					alis: .
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("."),
							),
						),
					),
				)))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`
				level1 {
					alis: ok @foo(bar: .)
				}
				`,
			mustPanic(
				mustParseFields(
					node(
						hasName("level1"),
						hasFields(
							node(
								hasAlias("alias"),
								hasName("ok"),
							),
						),
					),
				)))
	})

	// parseFieldsDefinition

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
				node(
					hasName("age"),
					nodeType(
						hasTypeName("Int"),
					),
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
				node(
					hasName("age"),
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

	// parseFragmentDefinition

	t.Run("simple fragment definition", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType @rename(index: 3){
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 2,
						CharStart: 5,
						LineEnd:   4,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("fragment without optional directives", func(t *testing.T) {
		run(`
				fragment MyFragment on SomeType{
					name
				}`,
			mustParseFragmentDefinition(
				node(
					hasName("MyFragment"),
					hasTypeName("SomeType"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			))
	})
	t.Run("fragment with untyped inline fragment", func(t *testing.T) {
		run(`	fragment inlineFragment2 on Dog {
  						... @include(if: true) {
    						name
  						}
					}`,
			mustParseFragmentDefinition(
				node(
					hasTypeName("Dog"),
					hasInlineFragments(
						node(
							hasDirectives(
								node(
									hasName("include"),
								),
							),
							hasFields(
								node(
									hasName("name"),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid fragment 1", func(t *testing.T) {
		run(`
				fragment MyFragment SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 2", func(t *testing.T) {
		run(`
				fragment MyFragment un SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 3", func(t *testing.T) {
		run(`
				fragment 1337 on SomeType{
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on [SomeType! {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})
	t.Run("invalid fragment 4", func(t *testing.T) {
		run(`
				fragment Fields on SomeType @foo(bar: .) {
					name
				}`,
			mustPanic(mustParseFragmentDefinition()))
	})

	// parseFragmentSpread

	t.Run("with directive", func(t *testing.T) {
		run("firstFragment @rename(index: 3)",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   1,
						CharEnd:   32,
					}),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("firstFragment",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("on", mustPanic(mustParseFragmentSpread(node())))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("afragment @foo(bar: .)", mustPanic(mustParseFragmentSpread(node())))
	})

	// parseImplementsInterfaces

	t.Run("simple", func(t *testing.T) {
		run("implements Dogs",
			mustParseImplementsInterfaces("Dogs"),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("implements Dogs & Cats & Mice",
			mustParseImplementsInterfaces("Dogs", "Cats", "Mice"),
		)
	})
	t.Run("multiple without &", func(t *testing.T) {
		run("implements Dogs & Cats Mice",
			mustParseImplementsInterfaces("Dogs", "Cats"),
			mustParseLiteral(keyword.IDENT, "Mice"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("implement Dogs & Cats Mice",
			mustParseImplementsInterfaces(),
			mustParseLiteral(keyword.IDENT, "implement"),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("implements foo & .",
			mustPanic(mustParseImplementsInterfaces("foo", ".")),
		)
	})

	// parseInlineFragment

	t.Run("with nested selectionsets", func(t *testing.T) {
		run(`on Goland {
					... on GoWater {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustParseInlineFragments(
				node(
					hasTypeName("Goland"),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   7,
						CharEnd:   6,
					}),
					hasInlineFragments(
						node(
							hasTypeName("GoWater"),
							hasPosition(position.Position{
								LineStart: 2,
								CharStart: 6,
								LineEnd:   6,
								CharEnd:   7,
							}),
							hasInlineFragments(
								node(
									hasTypeName("GoAir"),
									hasPosition(position.Position{
										LineStart: 3,
										CharStart: 7,
										LineEnd:   5,
										CharEnd:   8,
									}),
									hasFields(
										node(
											hasName("go"),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("inline fragment without type condition", func(t *testing.T) {
		run(`	@include(if: true) {
    					name
					}`,
			mustParseInlineFragments(
				node(
					hasDirectives(
						node(
							hasName("include"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`on Goland {
					... on 1337 {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("\"Goland\""),
						hasInlineFragments(
							node(
								hasTypeName("1337"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`on Goland {
					... on GoWater @foo(bar: .) {
						... on GoAir {
							go
						}
					}
				}
				`,
			mustPanic(
				mustParseInlineFragments(
					node(
						hasTypeName("Goland"),
						hasInlineFragments(
							node(
								hasTypeName("GoWater"),
								hasInlineFragments(
									node(
										hasTypeName("GoAir"),
										hasFields(
											node(
												hasName("go"),
											),
										),
									),
								),
							),
						),
					),
				),
			))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`	on Goland {
						... on [Water] {
							waterField
						}
					}`, mustPanic(mustParseInlineFragments(node())))
	})

	// parseInputFieldsDefinition

	t.Run("simple input fields definition", func(t *testing.T) {
		run("{inputValue: Int}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   18,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run(" ", mustParseInputFieldsDefinition())
	})
	t.Run("multiple", func(t *testing.T) {
		run("{inputValue: Int, outputValue: String}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
					node(
						hasName("outputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   39,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run("inputValue: Int}",
			mustParseInputFieldsDefinition(),
			mustParseLiteral(keyword.IDENT, "inputValue"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("{{inputValue: Int}",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("{inputValue: Int",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})

	// parseInputObjectTypeDefinition

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
							node(hasName("name")),
							node(hasName("age")),
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

	// parseInputValueDefinitions

	t.Run("simple", func(t *testing.T) {
		run("inputValue: Int",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run("inputValue: Int = 2",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
				),
			),
		)
	})
	t.Run("with description", func(t *testing.T) {
		run(`"useful description"inputValue: Int = 2`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("useful description"),
					hasName("inputValue"),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   40,
					}),
				),
			),
		)
	})
	t.Run("multiple with descriptions and defaults", func(t *testing.T) {
		run(`"this is a inputValue"inputValue: Int = 2, "this is a outputValue"outputValue: String = "Out"`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("this is a inputValue"),
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   42,
					}),
				),
				node(
					hasDescription("this is a outputValue"),
					hasName("outputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 44,
						LineEnd:   1,
						CharEnd:   94,
					}),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`inputValue: Int @fromTop(to: "bottom") @fromBottom(to: "top")`,
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   62,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("inputValue. foo",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("inputValue: foo @bar(baz: .)",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("inputValue: foo = [1!",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})

	// parseInterfaceTypeDefinition

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

	// parseObjectTypeDefinition

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

	// parseOperationDefinition

	t.Run("simple", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
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
	t.Run("without directive", func(t *testing.T) {
		run(` query allGophers($color: String) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("without variables", func(t *testing.T) {
		run(`query allGophers@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("unnamed", func(t *testing.T) {
		run(`query ($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run(`{
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasFields(
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
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: [String!) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: String!) @foo(bar: .) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})

	// parseScalarTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("scalar JSON", mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
			),
		))
	})
	t.Run("with directives", func(t *testing.T) {
		run(`scalar JSON @fromTop(to: "bottom") @fromBottom(to: "top")`, mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
				hasDirectives(
					node(
						hasName("fromTop"),
					),
					node(
						hasName("fromBottom"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   58,
				}),
			),
		))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("scalar 1337",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("scalar JSON @foo(bar: .)",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("notscalar JSON",
			mustPanic(
				mustParseScalarTypeDefinition(),
			),
		)
	})

	// parseSchemaDefinition

	t.Run("simple", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema {
						query : Query	
						mutation : Mutation
						subscription : Subscription
						query: Query2 
					}`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema @fromTop(to: "bottom") @fromBottom(to: "top") {
						query: Query
						mutation: Mutation
						subscription: Subscription
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
				hasDirectives(
					node(
						hasName("fromTop"),
					),
					node(
						hasName("fromBottom"),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`schema  @foo(bar: .) { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`schema ( query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`schema { query. Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`schema { query: 1337 }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run(`schema { query: Query )`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 6", func(t *testing.T) {
		run(`notschema { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})

	// parseSelectionSet

	t.Run("simple", func(t *testing.T) {
		run(`{ foo }`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasName("foo"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   8,
				}),
			),
		))
	})
	t.Run("inline and fragment spreads", func(t *testing.T) {
		run(`{
					... on Goland
					...Air
					... on Water
				}`,
			mustParseSelectionSet(
				node(
					hasInlineFragments(
						node(
							hasTypeName("Goland"),
						),
						node(
							hasTypeName("Water"),
						),
					),
					hasFragmentSpreads(
						node(
							hasName("Air"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   5,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("mixed", func(t *testing.T) {
		run(`{
					... on Goland
					preferredName: originalName(isSet: true)
					... on Water
				}`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasAlias("preferredName"),
						hasName("originalName"),
						hasArguments(
							node(
								hasName("isSet"),
							),
						),
					),
				),
				hasInlineFragments(
					node(
						hasTypeName("Goland"),
					),
					node(
						hasTypeName("Water"),
					),
				),
			),
		))
	})
	t.Run("field with directives", func(t *testing.T) {
		run(`{
					preferredName: originalName(isSet: true) @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFields(
					node(
						hasAlias("preferredName"),
						hasName("originalName"),
						hasArguments(
							node(
								hasName("isSet"),
							),
						),
						hasDirectives(
							node(
								hasName("rename"),
							),
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
		))
	})
	t.Run("fragment with directive", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: 3)
				}`, mustParseSelectionSet(
			node(
				hasFragmentSpreads(
					node(
						hasName("firstFragment"),
						hasDirectives(
							node(
								hasName("rename"),
							),
						),
					),
				),
			),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run(`{
					...firstFragment @rename(index: .)
				}`,
			mustPanic(
				mustParseSelectionSet(
					node(
						hasFragmentSpreads(
							node(
								hasName("firstFragment"),
								hasDirectives(
									node(
										hasName("rename"),
									),
								),
							),
						),
					),
				),
			),
		)
	})

	// parseTypeSystemDefinition

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
					
					"this is a scalar"
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

	// parseUnionTypeDefinition

	t.Run("simple", func(t *testing.T) {
		run("union SearchResult = Photo | Person",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("multiple members", func(t *testing.T) {
		run("union SearchResult = Photo | Person | Car | Planet",
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with linebreaks", func(t *testing.T) {
		run(`union SearchResult = Photo 
										| Person 
										| Car 
										| Planet`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasUnionMemberTypes("Photo", "Person", "Car", "Planet"),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`union SearchResult @fromTop(to: "bottom") @fromBottom(to: "top") = Photo | Person`,
			mustParseUnionTypeDefinition(
				node(
					hasName("SearchResult"),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasUnionMemberTypes("Photo", "Person"),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("union 1337 = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("1337"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("union SearchResult @foo(bar: .) = Photo | Person",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("union SearchResult = Photo | Person | 1337",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person", "1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("union SearchResult = Photo | Person | \"Video\"",
			mustPanic(
				mustParseUnionTypeDefinition(
					node(
						hasName("SearchResult"),
						hasUnionMemberTypes("Photo", "Person"),
					),
				),
			),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run("notunion SearchResult = Photo | Person",
			mustPanic(mustParseUnionTypeDefinition()),
		)
	})

	// parseVariableDefinitions

	t.Run("simple", func(t *testing.T) {
		run("($foo : bar)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   12,
					}),
				),
			),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("($foo : bar $baz : bat)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeName("bat"),
					),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run(`($foo : bar! = "me" $baz : bat)`,
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("bar"),
						),
					),
					hasDefaultValue(
						hasValueType(document.ValueTypeString),
						hasByteSliceValue("me"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   20,
					}),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("bat"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 21,
						LineEnd:   1,
						CharEnd:   31,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("($foo : bar!",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("($foo . bar!)",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("($foo : bar! = . )",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("($foo : bar! = \"Baz! )",
			mustPanic(mustParseVariableDefinitions()))
	})

	// parseValue

	t.Run("int", func(t *testing.T) {
		run("1337", mustParseValue(
			document.ValueTypeInt,
			expectIntegerValue(1337),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   4},
			),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`, mustParseValue(
			document.ValueTypeString,
			expectByteSliceValue("foo"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("list", func(t *testing.T) {
		run("[1,3,3,7]", mustParseValue(
			document.ValueTypeList,
			expectListValue(
				expectIntegerValue(1),
				expectIntegerValue(3),
				expectIntegerValue(3),
				expectIntegerValue(7),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("mixed list", func(t *testing.T) {
		run(`[ 1	,"2" 3,,[	1	], { foo: 1337 } ]`,
			mustParseValue(
				document.ValueTypeList,
				expectListValue(
					expectIntegerValue(1),
					expectByteSliceValue("2"),
					expectIntegerValue(3),
					expectListValue(
						expectIntegerValue(1),
					),
					expectObjectValue(
						node(
							hasName("foo"),
							expectIntegerValue(1337),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   35,
				}),
			))
	})
	t.Run("object", func(t *testing.T) {
		run(`{foo: "bar"}`,
			mustParseValue(document.ValueTypeObject,
				expectObjectValue(
					node(
						hasName("foo"),
						expectByteSliceValue("bar"),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					LineEnd:   1,
					CharStart: 1,
					CharEnd:   13,
				}),
			))
	})
	t.Run("invalid object", func(t *testing.T) {
		run(`{foo. "bar"}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 2", func(t *testing.T) {
		run(`{foo: [String!}`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid object 3", func(t *testing.T) {
		run(`{foo: "bar" )`,
			mustPanic(
				mustParseValue(document.ValueTypeObject,
					expectObjectValue(
						node(
							hasName("foo"),
							expectByteSliceValue("bar"),
						),
					),
				),
			),
		)
	})
	t.Run("nested object", func(t *testing.T) {
		run(`{foo: {bar: "baz"}, someEnum: SOME_ENUM }`, mustParseValue(document.ValueTypeObject,
			expectObjectValue(
				node(
					hasName("foo"),
					expectObjectValue(
						node(
							hasName("bar"),
							expectByteSliceValue("baz"),
						),
					),
				),
				node(
					hasName("someEnum"),
					expectByteSliceValue("SOME_ENUM"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   42,
			}),
		))
	})
	t.Run("variable", func(t *testing.T) {
		run("$1337", mustParseValue(
			document.ValueTypeVariable,
			expectByteSliceValue("1337"),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("variable 2", func(t *testing.T) {
		run("$foo", mustParseValue(document.ValueTypeVariable,
			expectByteSliceValue("foo"),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 1,
				End:   4},
			),
		))
	})
	t.Run("variable 3", func(t *testing.T) {
		run("$_foo", mustParseValue(document.ValueTypeVariable, expectByteSliceValue("_foo")))
	})
	t.Run("invalid variable", func(t *testing.T) {
		run("$ foo", mustPanic(mustParseValue(document.ValueTypeVariable, expectByteSliceValue(" foo"))))
	})
	t.Run("float", func(t *testing.T) {
		run("13.37", mustParseValue(
			document.ValueTypeFloat,
			expectFloatValue(13.37),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   5},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   6,
			}),
		))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseValue(document.ValueTypeFloat, expectFloatValue(13.37))))
	})
	t.Run("boolean", func(t *testing.T) {
		run("true", mustParseValue(
			document.ValueTypeBoolean,
			expectBooleanValue(true),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   4},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})
	t.Run("boolean 2", func(t *testing.T) {
		run("false", mustParseValue(document.ValueTypeBoolean,
			expectBooleanValue(false),
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   5},
			),
		))
	})
	t.Run("string", func(t *testing.T) {
		run(`"foo"`,
			mustParseValue(document.ValueTypeString,
				expectByteSliceValue("foo"),
				expectByteSliceRef(document.ByteSliceReference{
					Start: 1,
					End:   4},
				),
			))
	})
	t.Run("string 2", func(t *testing.T) {
		run(`"""foo"""`, mustParseValue(document.ValueTypeString, expectByteSliceValue("foo")))
	})
	t.Run("null", func(t *testing.T) {
		run("null", mustParseValue(
			document.ValueTypeNull,
			expectByteSliceRef(document.ByteSliceReference{
				Start: 0,
				End:   0},
			),
			hasPosition(position.Position{
				LineStart: 1,
				LineEnd:   1,
				CharStart: 1,
				CharEnd:   5,
			}),
		))
	})

	// parseTypes

	t.Run("simple named", func(t *testing.T) {
		run("String", mustParseType(
			hasTypeKind(document.TypeKindNAMED),
			hasTypeName("String"),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   7,
			}),
		))
	})
	t.Run("named non null", func(t *testing.T) {
		run("String!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindNAMED),
				hasTypeName("String"),
			),
		))
	})
	t.Run("non null named list", func(t *testing.T) {
		run("[String!]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindNON_NULL),
				ofType(
					hasTypeKind(document.TypeKindNAMED),
					hasTypeName("String"),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   10,
			}),
		))
	})
	t.Run("non null named non null list", func(t *testing.T) {
		run("[String!]!", mustParseType(
			hasTypeKind(document.TypeKindNON_NULL),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
				),
			),
		))
	})
	t.Run("nested list", func(t *testing.T) {
		run("[[[String]!]]", mustParseType(
			hasTypeKind(document.TypeKindLIST),
			ofType(
				hasTypeKind(document.TypeKindLIST),
				ofType(
					hasTypeKind(document.TypeKindNON_NULL),
					ofType(
						hasTypeKind(document.TypeKindLIST),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
							hasPosition(position.Position{
								LineStart: 1,
								CharStart: 4,
								LineEnd:   1,
								CharEnd:   10,
							}),
						),
					),
				),
			),
			hasPosition(position.Position{
				LineStart: 1,
				CharStart: 1,
				LineEnd:   1,
				CharEnd:   14,
			}),
		))
	})
	t.Run("invalid", func(t *testing.T) {
		run("[\"String\"]",
			mustPanic(
				mustParseType(
					hasTypeKind(document.TypeKindLIST),
					ofType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
			),
		)
	})

	// parsePeekedFloatValue
	t.Run("valid float", func(t *testing.T) {
		run("", mustParseFloatValue(t, "13.37", 13.37))
	})
	t.Run("invalid float", func(t *testing.T) {
		run("1.3.3.7", mustPanic(mustParseFloatValue(t, "1.3.3.7", 13.37)))
	})

	// newErrInvalidType

	t.Run("newErrInvalidType", func(t *testing.T) {
		want := "parser:a:invalidType - expected 'b', got 'c' @ 1:3-2:4"
		got := newErrInvalidType(position.Position{1, 2, 3, 4}, "a", "b", "c").Error()

		if want != got {
			t.Fatalf("newErrInvalidType: \nwant: %s\ngot: %s", want, got)
		}
	})

	// manual ast mod

	t.Run("manual ast modifications", func(t *testing.T) {

	})
}

func TestParser_ParseExecutableDefinition(t *testing.T) {
	parser := NewParser()
	input := make([]byte, 65536)
	err := parser.ParseTypeSystemDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}

	parser = NewParser()

	err = parser.ParseExecutableDefinition(input)
	if err == nil {
		t.Fatal("want err, got nil")
	}
}

func TestParser_CachedByteSlice(t *testing.T) {
	parser := NewParser()
	if parser.CachedByteSlice(-1) != nil {
		panic("want nil")
	}
}

func TestParser_putListValue(t *testing.T) {
	parser := NewParser()

	value, valueIndex := parser.makeValue()
	value.ValueType = document.ValueTypeInt
	value.Reference = parser.putInteger(1234)
	parser.putValue(value, valueIndex)

	var listValueIndex int
	var listValueIndex2 int

	listValue := parser.makeListValue(&listValueIndex)
	listValue2 := parser.makeListValue(&listValueIndex2)
	listValue = append(listValue, valueIndex)
	listValue2 = append(listValue2, valueIndex)

	parser.putListValue(listValue, &listValueIndex)
	parser.putListValue(listValue2, &listValueIndex2)

	if listValueIndex != listValueIndex2 {
		panic("expect lists to be merged")
	}

	if len(parser.ParsedDefinitions.ListValues) != 1 {
		panic("want duplicate to be deleted")
	}
}

func TestParser_putObjectValue(t *testing.T) {
	parser := NewParser()
	if err := parser.l.SetTypeSystemInput([]byte("foo bar")); err != nil {
		panic(err)
	}

	var iFoo document.Value
	var iBar document.Value
	parser.parsePeekedByteSlice(&iFoo)
	parser.parsePeekedByteSlice(&iBar)

	value1, iValue1 := parser.makeValue()
	value1.ValueType = document.ValueTypeInt
	value1.Reference = parser.putInteger(1234)
	parser.putValue(value1, iValue1)

	value2, iValue2 := parser.makeValue()
	value2.ValueType = document.ValueTypeInt
	value2.Reference = parser.putInteger(1234)
	parser.putValue(value2, iValue2)

	field1 := parser.putObjectField(document.ObjectField{
		Name:  iFoo.Reference,
		Value: iValue1,
	})

	field3 := parser.putObjectField(document.ObjectField{
		Name:  iFoo.Reference,
		Value: iValue1,
	})

	if field1 != field3 {
		panic("want identical fields to be merged")
	}

	field2 := parser.putObjectField(document.ObjectField{
		Name:  iBar.Reference,
		Value: iValue2,
	})

	var iObjectValue1 int
	objectValue1 := parser.makeObjectValue(&iObjectValue1)
	objectValue1 = append(objectValue1, field1, field2)

	var iObjectValue2 int
	objectValue2 := parser.makeObjectValue(&iObjectValue2)
	objectValue2 = append(objectValue2, field1, field2)

	parser.putObjectValue(objectValue1, &iObjectValue1)
	parser.putObjectValue(objectValue2, &iObjectValue2)

	if iObjectValue1 != iObjectValue2 {
		panic("expected object values to merge")
	}

	if len(parser.ParsedDefinitions.ObjectValues) != 1 {
		panic("want duplicated to be deleted")
	}
}

func TestParser_Starwars(t *testing.T) {

	inputFileName := "../../starwars.schema.graphql"
	fixtureFileName := "type_system_definition_parsed_starwars"

	parser := NewParser(WithPoolSize(2), WithMinimumSliceSize(2))

	starwarsSchema, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	err = parser.ParseTypeSystemDefinition(starwarsSchema)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.TypeSystemDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func TestParser_IntrospectionQuery(t *testing.T) {

	inputFileName := "./testdata/introspectionquery.graphql"
	fixtureFileName := "type_system_definition_parsed_introspection"

	inputFileData, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		t.Fatal(err)
	}

	parser := NewParser()
	err = parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	err = parser.ParseExecutableDefinition(inputFileData)
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes, err := json.MarshalIndent(parser.ParsedDefinitions.ExecutableDefinition, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, []byte("\n\n")...)

	parserData, err := json.MarshalIndent(parser, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	jsonBytes = append(jsonBytes, parserData...)

	goldie.Assert(t, fixtureFileName, jsonBytes)
	if t.Failed() {

		fixtureData, err := ioutil.ReadFile(fmt.Sprintf("./fixtures/%s.golden", fixtureFileName))
		if err != nil {
			log.Fatal(err)
		}

		diffview.NewGoland().DiffViewBytes(fixtureFileName, fixtureData, jsonBytes)
	}
}

func BenchmarkParser(b *testing.B) {

	b.ReportAllocs()

	parser := NewParser()

	testData, err := ioutil.ReadFile("./testdata/introspectionquery.graphql")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {

		err := parser.ParseExecutableDefinition(testData)
		if err != nil {
			b.Fatal(err)
		}

	}

}
