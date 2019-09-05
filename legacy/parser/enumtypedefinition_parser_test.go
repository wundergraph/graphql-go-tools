package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseEnumTypeDefinition(t *testing.T) {
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
					node(hasName("WEST")),
					node(hasName("SOUTH")),
					node(hasName("EAST")),
					node(hasName("NORTH")),
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
					node(hasName("WEST"), hasDescription("describes west")),
					node(hasName("SOUTH"), hasDescription("describes south")),
					node(hasName("EAST"), hasDescription("describes east")),
					node(hasName("NORTH"), hasDescription("describes north")),
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
				node(hasName("WEST"), hasDescription("describes west")),
				node(hasName("SOUTH"), hasDescription("describes south")),
				node(hasName("EAST"), hasDescription("describes east")),
				node(hasName("NORTH"), hasDescription("describes north")),
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
}
