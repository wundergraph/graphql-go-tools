package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseArgumentsDefinition(t *testing.T) {
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
}
