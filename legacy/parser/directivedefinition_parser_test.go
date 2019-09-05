package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseDirectiveDefinition(t *testing.T) {
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
}
